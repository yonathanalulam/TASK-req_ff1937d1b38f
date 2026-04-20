package lakehouse

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/eagle-point/service-portal/internal/apierr"
	"github.com/eagle-point/service-portal/internal/auth"
)

// Handler exposes lakehouse catalog + lineage + legal-hold endpoints.
type Handler struct {
	svc *Service
}

// NewHandler creates a Handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// ─── Catalog ─────────────────────────────────────────────────────────────────

func (h *Handler) ListCatalog(c *gin.Context) {
	sourceID, _ := strconv.ParseUint(c.Query("source_id"), 10, 64)
	layer := c.Query("layer")
	limit, _ := strconv.Atoi(c.Query("limit"))
	items, err := h.svc.ListCatalog(c.Request.Context(), sourceID, layer, limit)
	if err != nil {
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *Handler) GetCatalog(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		apierr.BadRequest(c, "invalid_param", "id must be a positive integer")
		return
	}
	m, err := h.svc.GetMetadata(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			apierr.NotFound(c, "metadata")
			return
		}
		apierr.InternalError(c)
		return
	}
	// Include immediate lineage chain
	lineage, _ := h.svc.Lineage(c.Request.Context(), id)
	c.JSON(http.StatusOK, gin.H{"metadata": m, "lineage": lineage})
}

func (h *Handler) GetLineage(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		apierr.BadRequest(c, "invalid_param", "id must be a positive integer")
		return
	}
	graph, err := h.svc.Lineage(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			apierr.NotFound(c, "metadata")
			return
		}
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"lineage": graph})
}

// ─── Lifecycle trigger ───────────────────────────────────────────────────────

// Default retention policy: 90 days bronze-to-archive, 18 months archive-to-purge.
// Matches the operational policy documented in the prompt.
const (
	DefaultArchiveDays = 90
	DefaultPurgeDays   = 18 * 30 // approx 18 months
)

// AdminRunLifecycle triggers a single lifecycle sweep synchronously. The
// caller can pass archive_days / purge_days query params; both default to the
// policy above. Returns counts of archived / purged / held rows.
func (h *Handler) AdminRunLifecycle(c *gin.Context) {
	archiveDays, _ := strconv.Atoi(c.Query("archive_days"))
	purgeDays, _ := strconv.Atoi(c.Query("purge_days"))
	if archiveDays <= 0 {
		archiveDays = DefaultArchiveDays
	}
	if purgeDays <= 0 {
		purgeDays = DefaultPurgeDays
	}
	res, err := h.svc.RunLifecycle(c.Request.Context(), archiveDays, purgeDays)
	if err != nil {
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"lifecycle": res, "archive_days": archiveDays, "purge_days": purgeDays})
}

// ─── Legal holds ─────────────────────────────────────────────────────────────

type holdRequest struct {
	SourceID *uint64 `json:"source_id"`
	JobID    *uint64 `json:"job_id"`
	Reason   string  `json:"reason" binding:"required"`
}

func (h *Handler) AdminListHolds(c *gin.Context) {
	items, err := h.svc.ListActiveHolds(c.Request.Context())
	if err != nil {
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"holds": items})
}

func (h *Handler) AdminPlaceHold(c *gin.Context) {
	var req holdRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.BadRequest(c, "validation_error", err.Error())
		return
	}
	if req.SourceID == nil && req.JobID == nil {
		apierr.UnprocessableEntity(c, "validation_error",
			"either source_id or job_id is required", nil)
		return
	}
	placedBy := c.GetUint64(auth.CtxUserID)
	hold, err := h.svc.PlaceHold(c.Request.Context(), req.SourceID, req.JobID, req.Reason, placedBy)
	if err != nil {
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"hold": hold})
}

func (h *Handler) AdminReleaseHold(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		apierr.BadRequest(c, "invalid_param", "id must be a positive integer")
		return
	}
	if err := h.svc.ReleaseHold(c.Request.Context(), id); err != nil {
		if errors.Is(err, ErrNotFound) {
			apierr.NotFound(c, "legal hold")
			return
		}
		apierr.InternalError(c)
		return
	}
	c.Status(http.StatusNoContent)
}
