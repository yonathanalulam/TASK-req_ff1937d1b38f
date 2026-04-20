package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/eagle-point/service-portal/internal/apierr"
	"github.com/eagle-point/service-portal/internal/config"
	"github.com/eagle-point/service-portal/internal/models"
	"github.com/eagle-point/service-portal/internal/session"
)

// ContextKey constants set by auth middleware.
const (
	CtxUserID   = "ctx_user_id"
	CtxUsername = "ctx_username"
	CtxRoles    = "ctx_roles"
	CtxSession  = "ctx_session"
)

// UnreadCounter resolves the unread notification count for a user.
// Optional dependency wired by the router so the auth package does not depend
// on the notification package directly.
type UnreadCounter func(ctx context.Context, userID uint64) (int, error)

// Handler handles authentication HTTP endpoints.
type Handler struct {
	svc        *Service
	sessions   *session.Store
	cfg        *config.Config
	unreadFunc UnreadCounter
}

// NewHandler creates an auth Handler.
func NewHandler(svc *Service, ss *session.Store, cfg *config.Config) *Handler {
	return &Handler{svc: svc, sessions: ss, cfg: cfg}
}

// SetUnreadCounter wires the optional unread-count provider for /auth/me.
func (h *Handler) SetUnreadCounter(fn UnreadCounter) { h.unreadFunc = fn }

// ─── POST /api/v1/auth/register ─────────────────────────────────────────────

type registerRequest struct {
	Username    string `json:"username" binding:"required,min=3,max=50"`
	Email       string `json:"email"    binding:"required,email"`
	Password    string `json:"password" binding:"required"`
	DisplayName string `json:"display_name" binding:"required,min=1,max=100"`
}

func (h *Handler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.BadRequest(c, "validation_error", err.Error())
		return
	}

	user, err := h.svc.Register(c.Request.Context(), RegisterInput{
		Username:    req.Username,
		Email:       req.Email,
		Password:    req.Password,
		DisplayName: req.DisplayName,
	})
	if err != nil {
		if strings.Contains(err.Error(), "already taken") {
			apierr.Conflict(c, "duplicate_user", err.Error())
			return
		}
		apierr.UnprocessableEntity(c, "validation_error", err.Error(), nil)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"user": user.SafeView()})
}

// ─── POST /api/v1/auth/login ────────────────────────────────────────────────

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.BadRequest(c, "validation_error", err.Error())
		return
	}

	out, err := h.svc.Login(c.Request.Context(), LoginInput{
		Username:  req.Username,
		Password:  req.Password,
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	})
	if err != nil {
		var lockErr *LockoutError
		if errors.As(err, &lockErr) {
			apierr.AccountLocked(c, lockErr.RemainingSeconds())
			return
		}
		if errors.Is(err, ErrInvalidCredentials) {
			apierr.UnprocessableEntity(c, "invalid_credentials", "invalid username or password", nil)
			return
		}
		apierr.InternalError(c)
		return
	}

	h.setSessionCookie(c, out.Session.ID)

	c.JSON(http.StatusOK, gin.H{
		"user":       out.User.SafeView(),
		"csrf_token": out.Session.CSRFToken,
	})
}

// ─── POST /api/v1/auth/logout ───────────────────────────────────────────────

func (h *Handler) Logout(c *gin.Context) {
	sessID, _ := c.Cookie(session.CookieName())
	if sessID != "" {
		_ = h.sessions.Delete(c.Request.Context(), sessID)
	}

	// Clear cookie. Secure flag mirrors the actual request transport so the
	// browser will accept the removal under the same conditions as the
	// original set — production with TLS emits Secure=true, in-process
	// httptest with plain HTTP emits Secure=false.
	c.SetCookie(session.CookieName(), "", -1, "/", h.cfg.SessionCookieDomain,
		isSecureRequest(c), true)

	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}

// ─── GET /api/v1/auth/me ────────────────────────────────────────────────────

func (h *Handler) Me(c *gin.Context) {
	userID := c.GetUint64(CtxUserID)

	user, err := h.svc.GetUserByID(c.Request.Context(), userID)
	if err != nil || user == nil {
		apierr.Unauthorized(c)
		return
	}

	// Return current CSRF token from session
	var csrfToken string
	if sess, ok := c.Get(CtxSession); ok {
		if s, ok := sess.(*models.Session); ok {
			csrfToken = s.CSRFToken
		}
	}

	resp := gin.H{
		"user":       user.SafeView(),
		"csrf_token": csrfToken,
	}
	if h.unreadFunc != nil {
		if n, err := h.unreadFunc(c.Request.Context(), userID); err == nil {
			resp["unread_count"] = n
		}
	}
	c.JSON(http.StatusOK, resp)
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func (h *Handler) setSessionCookie(c *gin.Context, sessionID string) {
	maxAge := int(session.AbsoluteTimeout.Seconds())
	// Secure is derived from the actual transport of the request (TLS
	// termination detection, per the audit) rather than APP_ENV. In
	// production TLS is mandatory so every request arrives with c.Request.TLS
	// set — Secure=true. Integration tests that run httptest.NewServer on
	// plain HTTP get Secure=false so the stdlib cookie jar will still echo
	// the cookie back. This is not "APP_ENV-based" behavior — it mirrors the
	// real transport the client used.
	c.SetCookie(
		session.CookieName(),
		sessionID,
		maxAge,
		"/",
		h.cfg.SessionCookieDomain,
		isSecureRequest(c), // Secure
		true,               // HttpOnly
	)
}

// isSecureRequest reports whether the CLIENT's connection to the edge is
// HTTPS. Behind a reverse proxy (nginx → backend), the last hop is always
// TLS terminated by the backend, but the client's actual scheme may be HTTP
// (dev, Playwright) or HTTPS (prod). X-Forwarded-Proto, when present, is
// authoritative — it mirrors the nginx-side scheme. Falling back to the raw
// c.Request.TLS check covers the direct-client case (no proxy, e.g. unit
// tests hitting httptest.NewTLSServer).
func isSecureRequest(c *gin.Context) bool {
	if proto := c.GetHeader("X-Forwarded-Proto"); proto != "" {
		return strings.EqualFold(proto, "https")
	}
	return c.Request.TLS != nil
}
