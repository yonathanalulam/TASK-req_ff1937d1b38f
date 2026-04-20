package notification

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/eagle-point/service-portal/internal/apierr"
	"github.com/eagle-point/service-portal/internal/auth"
)

// Handler exposes notification + template HTTP endpoints.
type Handler struct {
	svc *Service
}

// NewHandler creates a Handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// ─── User-facing notification API ────────────────────────────────────────────

// List GET /api/v1/users/me/notifications
func (h *Handler) List(c *gin.Context) {
	userID := c.GetUint64(auth.CtxUserID)
	readState := c.Query("read") // "read", "unread", or empty
	cursor, _ := strconv.ParseUint(c.Query("cursor"), 10, 64)
	limit, _ := strconv.Atoi(c.Query("limit"))

	// Support ?read=true|false shorthand
	switch readState {
	case "true":
		readState = "read"
	case "false":
		readState = "unread"
	}

	page, err := h.svc.List(c.Request.Context(), userID, readState, cursor, limit)
	if err != nil {
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, page)
}

// MarkRead PATCH /api/v1/users/me/notifications/:id/read
func (h *Handler) MarkRead(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		apierr.BadRequest(c, "invalid_param", "id must be a positive integer")
		return
	}
	userID := c.GetUint64(auth.CtxUserID)
	if err := h.svc.MarkRead(c.Request.Context(), id, userID); err != nil {
		if errors.Is(err, ErrNotFound) {
			apierr.NotFound(c, "notification")
			return
		}
		apierr.InternalError(c)
		return
	}
	c.Status(http.StatusNoContent)
}

// MarkAllRead PATCH /api/v1/users/me/notifications/read-all
func (h *Handler) MarkAllRead(c *gin.Context) {
	userID := c.GetUint64(auth.CtxUserID)
	n, err := h.svc.MarkAllRead(c.Request.Context(), userID)
	if err != nil {
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"marked_read": n})
}

// Outbox GET /api/v1/users/me/notifications/outbox
func (h *Handler) Outbox(c *gin.Context) {
	userID := c.GetUint64(auth.CtxUserID)
	limit, _ := strconv.Atoi(c.Query("limit"))

	out, err := h.svc.ListOutbox(c.Request.Context(), userID, limit)
	if err != nil {
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

// UnreadCount GET /api/v1/users/me/notifications/unread-count
func (h *Handler) UnreadCount(c *gin.Context) {
	userID := c.GetUint64(auth.CtxUserID)
	n, err := h.svc.UnreadCount(c.Request.Context(), userID)
	if err != nil {
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"unread_count": n})
}

// ─── Admin template CRUD ─────────────────────────────────────────────────────

func (h *Handler) AdminListTemplates(c *gin.Context) {
	items, err := h.svc.ListTemplates(c.Request.Context())
	if err != nil {
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"templates": items})
}

type upsertTemplateRequest struct {
	TitleTemplate string `json:"title_template" binding:"required"`
	BodyTemplate  string `json:"body_template"  binding:"required"`
}

func (h *Handler) AdminUpsertTemplate(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		apierr.BadRequest(c, "invalid_param", "code is required")
		return
	}
	var req upsertTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.BadRequest(c, "validation_error", err.Error())
		return
	}
	t, err := h.svc.UpsertTemplate(c.Request.Context(), code, req.TitleTemplate, req.BodyTemplate)
	if err != nil {
		switch {
		case errors.Is(err, ErrTemplateParse):
			apierr.UnprocessableEntity(c, "template_parse_error", err.Error(), nil)
		case errors.Is(err, ErrValidation):
			apierr.UnprocessableEntity(c, "validation_error", err.Error(), nil)
		default:
			apierr.InternalError(c)
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"template": t})
}
