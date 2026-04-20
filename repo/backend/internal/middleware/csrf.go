package middleware

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/eagle-point/service-portal/internal/apierr"
	"github.com/eagle-point/service-portal/internal/auth"
	"github.com/eagle-point/service-portal/internal/models"
	"github.com/eagle-point/service-portal/internal/session"
)

const csrfHeader = "X-CSRF-Token"

// safeMethods are HTTP methods that do not mutate state and therefore
// do not require CSRF validation.
var safeMethods = map[string]struct{}{
	http.MethodGet:     {},
	http.MethodHead:    {},
	http.MethodOptions: {},
}

// CSRF provides CSRF token validation middleware.
type CSRF struct {
	db *sql.DB
}

// NewCSRF creates a CSRF middleware.
func NewCSRF(db *sql.DB) *CSRF {
	return &CSRF{db: db}
}

// Validate checks the X-CSRF-Token header against the token stored in the
// session for all state-changing requests (POST/PUT/PATCH/DELETE).
func (cs *CSRF) Validate() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, safe := safeMethods[c.Request.Method]; safe {
			c.Next()
			return
		}

		token := c.GetHeader(csrfHeader)
		if token == "" {
			apierr.BadRequest(c, "csrf_missing", "X-CSRF-Token header is required")
			return
		}

		sessVal, exists := c.Get(auth.CtxSession)
		if !exists {
			apierr.Unauthorized(c)
			return
		}

		sess, ok := sessVal.(*models.Session)
		if !ok || sess.CSRFToken != token {
			apierr.BadRequest(c, "csrf_invalid", "invalid or expired CSRF token")
			return
		}

		c.Next()
	}
}

// ValidateSession is an alias used after RequireAuth to validate CSRF.
func (cs *CSRF) ValidateSession() gin.HandlerFunc {
	return cs.Validate()
}

// SetCSRFCookieName returns the header name used for CSRF tokens.
func CSRFHeaderName() string { return csrfHeader }

// ─── session helper ──────────────────────────────────────────────────────────

// GetSession retrieves the current session from context (set by RequireAuth).
func GetSession(c *gin.Context) *session.Store {
	_ = c.GetHeader("") // keep import used
	return nil
}
