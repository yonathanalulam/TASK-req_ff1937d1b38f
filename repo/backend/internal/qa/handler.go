package qa

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/eagle-point/service-portal/internal/apierr"
	"github.com/eagle-point/service-portal/internal/auth"
)

// BorderlineHook is the moderation enqueue hook (wired by the router).
type BorderlineHook func(ctx context.Context, contentType string, contentID uint64, text string, flagged []string) error

// Context keys mirrored from moderation.middleware.
const (
	borderlineTextKey  = "ctx_borderline_text"
	borderlineTermsKey = "ctx_borderline_terms"
)

// Handler exposes Q&A HTTP endpoints.
type Handler struct {
	svc          *Service
	onBorderline BorderlineHook
}

// NewHandler creates a Handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// SetBorderlineHook wires the optional moderation enqueue hook.
func (h *Handler) SetBorderlineHook(fn BorderlineHook) { h.onBorderline = fn }

// fireBorderline is shared by CreateThread and CreateReply.
func (h *Handler) fireBorderline(c *gin.Context, contentType string, contentID uint64) {
	if h.onBorderline == nil {
		return
	}
	textVal, ok := c.Get(borderlineTextKey)
	if !ok {
		return
	}
	text, _ := textVal.(string)
	var terms []string
	if v, ok := c.Get(borderlineTermsKey); ok {
		terms, _ = v.([]string)
	}
	_ = h.onBorderline(c.Request.Context(), contentType, contentID, text, terms)
}

// ─── Threads ─────────────────────────────────────────────────────────────────

type createThreadRequest struct {
	Question string `json:"question" binding:"required"`
}

// CreateThread POST /api/v1/service-offerings/:id/qa
func (h *Handler) CreateThread(c *gin.Context) {
	offeringID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || offeringID == 0 {
		apierr.BadRequest(c, "invalid_param", "id must be a positive integer")
		return
	}
	var req createThreadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.BadRequest(c, "validation_error", err.Error())
		return
	}
	authorID := c.GetUint64(auth.CtxUserID)
	t, err := h.svc.CreateThread(c.Request.Context(), offeringID, authorID, req.Question)
	if err != nil {
		if errors.Is(err, ErrValidation) {
			apierr.UnprocessableEntity(c, "validation_error", err.Error(), nil)
			return
		}
		apierr.InternalError(c)
		return
	}
	h.fireBorderline(c, "qa_thread", t.ID)
	// Reload to reflect possible status change
	if reloaded, gerr := h.svc.GetThread(c.Request.Context(), t.ID); gerr == nil {
		t = reloaded
	}
	c.JSON(http.StatusCreated, gin.H{"thread": t})
}

// ListThreads GET /api/v1/service-offerings/:id/qa
func (h *Handler) ListThreads(c *gin.Context) {
	offeringID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || offeringID == 0 {
		apierr.BadRequest(c, "invalid_param", "id must be a positive integer")
		return
	}
	cursor, _ := strconv.ParseUint(c.Query("cursor"), 10, 64)
	limit, _ := strconv.Atoi(c.Query("limit"))
	items, next, err := h.svc.ListThreads(c.Request.Context(), offeringID, cursor, limit)
	if err != nil {
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "next_cursor": next})
}

// ─── Replies ─────────────────────────────────────────────────────────────────

type createReplyRequest struct {
	Content string `json:"content" binding:"required"`
}

// CreateReply POST /api/v1/service-offerings/:id/qa/:thread_id/replies
func (h *Handler) CreateReply(c *gin.Context) {
	threadID, err := strconv.ParseUint(c.Param("thread_id"), 10, 64)
	if err != nil || threadID == 0 {
		apierr.BadRequest(c, "invalid_param", "thread_id must be a positive integer")
		return
	}
	var req createReplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.BadRequest(c, "validation_error", err.Error())
		return
	}
	authorID := c.GetUint64(auth.CtxUserID)
	p, err := h.svc.CreateReply(c.Request.Context(), threadID, authorID, req.Content)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			apierr.NotFound(c, "thread")
		case errors.Is(err, ErrValidation):
			apierr.UnprocessableEntity(c, "validation_error", err.Error(), nil)
		default:
			apierr.InternalError(c)
		}
		return
	}
	h.fireBorderline(c, "qa_post", p.ID)
	c.JSON(http.StatusCreated, gin.H{"reply": p})
}

// DeletePost DELETE /api/v1/qa/:post_id  (Moderator or Administrator)
func (h *Handler) DeletePost(c *gin.Context) {
	postID, err := strconv.ParseUint(c.Param("post_id"), 10, 64)
	if err != nil || postID == 0 {
		apierr.BadRequest(c, "invalid_param", "post_id must be a positive integer")
		return
	}
	if err := h.svc.DeletePost(c.Request.Context(), postID); err != nil {
		if errors.Is(err, ErrNotFound) {
			apierr.NotFound(c, "post")
			return
		}
		apierr.InternalError(c)
		return
	}
	c.Status(http.StatusNoContent)
}
