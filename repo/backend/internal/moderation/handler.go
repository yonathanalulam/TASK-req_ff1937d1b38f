package moderation

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/eagle-point/service-portal/internal/apierr"
	"github.com/eagle-point/service-portal/internal/auth"
)

// Handler exposes moderation HTTP endpoints.
type Handler struct {
	svc *Service
}

// NewHandler creates a Handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// ─── Admin: sensitive term dictionary ────────────────────────────────────────

func (h *Handler) ListTerms(c *gin.Context) {
	items, err := h.svc.ListTerms(c.Request.Context())
	if err != nil {
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"terms": items})
}

type addTermRequest struct {
	Term  string `json:"term"  binding:"required"`
	Class string `json:"class" binding:"required"`
}

func (h *Handler) AddTerm(c *gin.Context) {
	var req addTermRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.BadRequest(c, "validation_error", err.Error())
		return
	}
	t, err := h.svc.AddTerm(c.Request.Context(), req.Term, req.Class)
	if err != nil {
		switch {
		case errors.Is(err, ErrDuplicate):
			apierr.Conflict(c, "duplicate", "term already exists")
		case errors.Is(err, ErrValidation):
			apierr.UnprocessableEntity(c, "validation_error", err.Error(), nil)
		default:
			apierr.InternalError(c)
		}
		return
	}
	c.JSON(http.StatusCreated, gin.H{"term": t})
}

func (h *Handler) DeleteTerm(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		apierr.BadRequest(c, "invalid_param", "id must be a positive integer")
		return
	}
	if err := h.svc.DeleteTerm(c.Request.Context(), id); err != nil {
		if errors.Is(err, ErrNotFound) {
			apierr.NotFound(c, "term")
			return
		}
		apierr.InternalError(c)
		return
	}
	c.Status(http.StatusNoContent)
}

// ─── Moderation queue ────────────────────────────────────────────────────────

func (h *Handler) ListQueue(c *gin.Context) {
	status := c.Query("status")
	items, err := h.svc.ListQueue(c.Request.Context(), status)
	if err != nil {
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

type decisionRequest struct {
	Reason string `json:"reason"`
}

func (h *Handler) Approve(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		apierr.BadRequest(c, "invalid_param", "id must be a positive integer")
		return
	}
	var req decisionRequest
	_ = c.ShouldBindJSON(&req)

	modID := c.GetUint64(auth.CtxUserID)
	it, err := h.svc.ApproveItem(c.Request.Context(), id, modID, req.Reason)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			apierr.NotFound(c, "queue item")
		case errors.Is(err, ErrValidation):
			apierr.UnprocessableEntity(c, "validation_error", err.Error(), nil)
		default:
			apierr.InternalError(c)
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"item": it})
}

func (h *Handler) Reject(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		apierr.BadRequest(c, "invalid_param", "id must be a positive integer")
		return
	}
	var req decisionRequest
	_ = c.ShouldBindJSON(&req)

	modID := c.GetUint64(auth.CtxUserID)
	it, freezeUntil, err := h.svc.RejectItem(c.Request.Context(), id, modID, req.Reason)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			apierr.NotFound(c, "queue item")
		case errors.Is(err, ErrValidation):
			apierr.UnprocessableEntity(c, "validation_error", err.Error(), nil)
		default:
			apierr.InternalError(c)
		}
		return
	}
	resp := gin.H{"item": it}
	if freezeUntil != nil {
		resp["freeze_until"] = freezeUntil
	}
	c.JSON(http.StatusOK, resp)
}

// ─── Actions + violations ────────────────────────────────────────────────────

func (h *Handler) ListActions(c *gin.Context) {
	modID, _ := strconv.ParseUint(c.Query("moderator_id"), 10, 64)
	limit, _ := strconv.Atoi(c.Query("limit"))
	items, err := h.svc.ListActions(c.Request.Context(), modID, limit)
	if err != nil {
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"actions": items})
}

func (h *Handler) ListUserViolations(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("user_id"), 10, 64)
	if err != nil || userID == 0 {
		apierr.BadRequest(c, "invalid_param", "user_id must be a positive integer")
		return
	}
	items, err := h.svc.ListViolations(c.Request.Context(), userID)
	if err != nil {
		apierr.InternalError(c)
		return
	}
	// Include current freeze status
	until, _ := h.svc.IsUserFrozen(c.Request.Context(), userID)
	resp := gin.H{"violations": items}
	if until != nil {
		resp["freeze_until"] = until
	}
	c.JSON(http.StatusOK, resp)
}
