package privacy

import (
	"errors"
	"net/http"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/eagle-point/service-portal/internal/apierr"
	"github.com/eagle-point/service-portal/internal/auth"
)

// Handler exposes user-facing privacy endpoints.
type Handler struct {
	svc *Service
}

// NewHandler creates a Handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// ─── Export ──────────────────────────────────────────────────────────────────

// RequestExport POST /api/v1/users/me/export-request
func (h *Handler) RequestExport(c *gin.Context) {
	userID := c.GetUint64(auth.CtxUserID)
	req, err := h.svc.RequestExport(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrAlreadyPending) {
			apierr.Conflict(c, "already_pending", "an export request is already in progress")
			return
		}
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"export_request": req})
}

// Status GET /api/v1/users/me/export-request/status
func (h *Handler) ExportStatus(c *gin.Context) {
	userID := c.GetUint64(auth.CtxUserID)
	req, err := h.svc.GetActiveExport(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			apierr.NotFound(c, "export request")
			return
		}
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"export_request": req})
}

// Download GET /api/v1/users/me/export-request/download
func (h *Handler) Download(c *gin.Context) {
	userID := c.GetUint64(auth.CtxUserID)
	req, err := h.svc.GetActiveExport(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			apierr.NotFound(c, "export request")
			return
		}
		apierr.InternalError(c)
		return
	}
	if req.Status != "ready" {
		apierr.UnprocessableEntity(c, "not_ready", "export not ready yet", nil)
		return
	}
	if req.FilePath == "" {
		apierr.InternalError(c)
		return
	}
	if _, err := os.Stat(req.FilePath); err != nil {
		apierr.NotFound(c, "export file")
		return
	}

	// Stream the file
	c.Header("Content-Disposition", `attachment; filename="user-export-`+strconv.FormatUint(req.ID, 10)+`.zip"`)
	c.Header("Content-Type", "application/zip")
	c.File(req.FilePath)

	// Mark downloaded after streaming. Errors are non-fatal.
	_ = h.svc.MarkDownloaded(c.Request.Context(), req.ID)
}

// ─── Deletion ────────────────────────────────────────────────────────────────

type deletionRequest struct {
	Confirm string `json:"confirm" binding:"required"`
}

// RequestDeletion POST /api/v1/users/me/deletion-request
// Requires `confirm: "DELETE"` in the body to guard against accidents.
func (h *Handler) RequestDeletion(c *gin.Context) {
	var req deletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.BadRequest(c, "validation_error", err.Error())
		return
	}
	if req.Confirm != "DELETE" {
		apierr.UnprocessableEntity(c, "confirm_required",
			`type "DELETE" in the confirm field to proceed`, nil)
		return
	}
	userID := c.GetUint64(auth.CtxUserID)
	dr, err := h.svc.RequestDeletion(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrAlreadyPending) {
			apierr.Conflict(c, "already_pending", "a deletion request is already pending")
			return
		}
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"deletion_request": dr})
}

// DeletionStatus GET /api/v1/users/me/deletion-request/status
func (h *Handler) DeletionStatus(c *gin.Context) {
	userID := c.GetUint64(auth.CtxUserID)
	dr, err := h.svc.GetActiveDeletion(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			apierr.NotFound(c, "deletion request")
			return
		}
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deletion_request": dr})
}

// AdminHardDelete DELETE /api/v1/admin/users/:id
func (h *Handler) AdminHardDelete(c *gin.Context) {
	targetID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || targetID == 0 {
		apierr.BadRequest(c, "invalid_param", "id must be a positive integer")
		return
	}
	adminID := c.GetUint64(auth.CtxUserID)
	if err := h.svc.AdminHardDelete(c.Request.Context(), targetID, adminID); err != nil {
		apierr.InternalError(c)
		return
	}
	c.Status(http.StatusNoContent)
}
