package shipping

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/eagle-point/service-portal/internal/apierr"
)

// Handler exposes shipping HTTP endpoints.
type Handler struct {
	svc *Service
}

// NewHandler creates a Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// ─── Regions ─────────────────────────────────────────────────────────────────

func (h *Handler) ListRegions(c *gin.Context) {
	regions, err := h.svc.ListRegions(c.Request.Context())
	if err != nil {
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"regions": regions})
}

type createRegionRequest struct {
	Name       string `json:"name"        binding:"required"`
	CutoffTime string `json:"cutoff_time"`
	Timezone   string `json:"timezone"`
}

func (h *Handler) CreateRegion(c *gin.Context) {
	var req createRegionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.BadRequest(c, "validation_error", err.Error())
		return
	}
	region, err := h.svc.CreateRegion(c.Request.Context(), CreateRegionInput{
		Name: req.Name, CutoffTime: req.CutoffTime, Timezone: req.Timezone,
	})
	if err != nil {
		apierr.UnprocessableEntity(c, "validation_error", err.Error(), nil)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"region": region})
}

// ─── Templates ───────────────────────────────────────────────────────────────

func (h *Handler) ListTemplates(c *gin.Context) {
	regionID, _ := strconv.ParseUint(c.Query("region_id"), 10, 64)
	templates, err := h.svc.ListTemplates(c.Request.Context(), regionID)
	if err != nil {
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"templates": templates})
}

type templateRequest struct {
	RegionID       uint64  `json:"region_id"        binding:"required"`
	DeliveryMethod string  `json:"delivery_method"  binding:"required"`
	MinWeightKg    float64 `json:"min_weight_kg"`
	MaxWeightKg    float64 `json:"max_weight_kg"`
	MinQuantity    int     `json:"min_quantity"`
	MaxQuantity    int     `json:"max_quantity"`
	FeeAmount      float64 `json:"fee_amount"`
	Currency       string  `json:"currency"`
	LeadTimeHours  int     `json:"lead_time_hours"`
	WindowHours    int     `json:"window_hours"`
}

func (h *Handler) CreateTemplate(c *gin.Context) {
	var req templateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.BadRequest(c, "validation_error", err.Error())
		return
	}
	tmpl, err := h.svc.CreateTemplate(c.Request.Context(), toCreateTemplateInput(req))
	if err != nil {
		apierr.UnprocessableEntity(c, "validation_error", err.Error(), nil)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"template": tmpl})
}

func (h *Handler) UpdateTemplate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		apierr.BadRequest(c, "invalid_param", "id must be a positive integer")
		return
	}
	var req templateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.BadRequest(c, "validation_error", err.Error())
		return
	}
	tmpl, err := h.svc.UpdateTemplate(c.Request.Context(), id, toCreateTemplateInput(req))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			apierr.NotFound(c, "template")
			return
		}
		apierr.UnprocessableEntity(c, "validation_error", err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"template": tmpl})
}

// ─── Estimate ────────────────────────────────────────────────────────────────

type estimateRequest struct {
	RegionID       uint64    `json:"region_id"        binding:"required"`
	WeightKg       float64   `json:"weight_kg"`
	Quantity       int       `json:"quantity"`
	DeliveryMethod string    `json:"delivery_method"  binding:"required"`
	RequestedAt    time.Time `json:"requested_at"`
}

func (h *Handler) Estimate(c *gin.Context) {
	var req estimateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.BadRequest(c, "validation_error", err.Error())
		return
	}
	if req.RequestedAt.IsZero() {
		req.RequestedAt = time.Now().UTC()
	}
	if req.Quantity <= 0 {
		req.Quantity = 1
	}

	result, err := h.svc.Estimate(c.Request.Context(), EstimateInput{
		RegionID:       req.RegionID,
		WeightKg:       req.WeightKg,
		Quantity:       req.Quantity,
		DeliveryMethod: req.DeliveryMethod,
		RequestedAt:    req.RequestedAt,
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			apierr.NotFound(c, "shipping region")
			return
		}
		if errors.Is(err, ErrNoTemplate) {
			apierr.UnprocessableEntity(c, "no_template",
				"no shipping template found for the given parameters", nil)
			return
		}
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, result)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func toCreateTemplateInput(req templateRequest) CreateTemplateInput {
	return CreateTemplateInput{
		RegionID: req.RegionID, DeliveryMethod: req.DeliveryMethod,
		MinWeightKg: req.MinWeightKg, MaxWeightKg: req.MaxWeightKg,
		MinQuantity: req.MinQuantity, MaxQuantity: req.MaxQuantity,
		FeeAmount: req.FeeAmount, Currency: req.Currency,
		LeadTimeHours: req.LeadTimeHours, WindowHours: req.WindowHours,
	}
}
