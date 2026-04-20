package catalog

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/eagle-point/service-portal/internal/apierr"
	"github.com/eagle-point/service-portal/internal/auth"
	"github.com/eagle-point/service-portal/internal/models"
	"github.com/eagle-point/service-portal/internal/profile"
)

// Handler exposes catalog HTTP endpoints.
type Handler struct {
	svc        *Service
	profileSvc *profile.Service
}

// NewHandler creates a Handler.
func NewHandler(svc *Service, profileSvc *profile.Service) *Handler {
	return &Handler{svc: svc, profileSvc: profileSvc}
}

// ─── Categories ──────────────────────────────────────────────────────────────

func (h *Handler) ListCategories(c *gin.Context) {
	cats, err := h.svc.ListCategories(c.Request.Context())
	if err != nil {
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"categories": cats})
}

type categoryRequest struct {
	Name                  string `json:"name"  binding:"required"`
	Slug                  string `json:"slug"  binding:"required"`
	Description           string `json:"description"`
	ResponseTimeMinutes   int    `json:"response_time_minutes"`
	CompletionTimeMinutes int    `json:"completion_time_minutes"`
}

func (h *Handler) CreateCategory(c *gin.Context) {
	var req categoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.BadRequest(c, "validation_error", err.Error())
		return
	}
	cat, err := h.svc.CreateCategory(c.Request.Context(), CreateCategoryInput{
		Name: req.Name, Slug: req.Slug, Description: req.Description,
		ResponseTimeMinutes:   req.ResponseTimeMinutes,
		CompletionTimeMinutes: req.CompletionTimeMinutes,
	})
	if err != nil {
		apierr.UnprocessableEntity(c, "validation_error", err.Error(), nil)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"category": cat})
}

func (h *Handler) UpdateCategory(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		apierr.BadRequest(c, "invalid_param", "id must be a positive integer")
		return
	}
	var req categoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.BadRequest(c, "validation_error", err.Error())
		return
	}
	cat, err := h.svc.UpdateCategory(c.Request.Context(), id, CreateCategoryInput{
		Name: req.Name, Description: req.Description,
		ResponseTimeMinutes:   req.ResponseTimeMinutes,
		CompletionTimeMinutes: req.CompletionTimeMinutes,
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			apierr.NotFound(c, "category")
			return
		}
		apierr.UnprocessableEntity(c, "validation_error", err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"category": cat})
}

func (h *Handler) DeleteCategory(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		apierr.BadRequest(c, "invalid_param", "id must be a positive integer")
		return
	}
	if err := h.svc.DeleteCategory(c.Request.Context(), id); err != nil {
		if errors.Is(err, ErrNotFound) {
			apierr.NotFound(c, "category")
			return
		}
		apierr.InternalError(c)
		return
	}
	c.Status(http.StatusNoContent)
}

// ─── Offerings ───────────────────────────────────────────────────────────────

func (h *Handler) ListOfferings(c *gin.Context) {
	categoryID := parseQueryUint64(c, "category_id")
	active := -1 // -1 = all
	if v := c.Query("active"); v == "true" {
		active = 1
	} else if v == "false" {
		active = 0
	}
	cursor := parseQueryUint64(c, "cursor")
	limit := parseQueryInt(c, "limit", 20)

	page, err := h.svc.ListOfferings(c.Request.Context(), OfferingFilter{
		CategoryID: categoryID,
		Active:     active,
	}, cursor, limit)
	if err != nil {
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, page)
}

func (h *Handler) GetOffering(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		apierr.BadRequest(c, "invalid_param", "id must be a positive integer")
		return
	}

	offering, err := h.svc.GetOffering(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			apierr.NotFound(c, "offering")
			return
		}
		apierr.InternalError(c)
		return
	}

	// Record browsing history for regular users (fire-and-forget)
	if userID := c.GetUint64(auth.CtxUserID); userID > 0 {
		if rolesVal, exists := c.Get(auth.CtxRoles); exists {
			if roles, ok := rolesVal.([]string); ok {
				for _, r := range roles {
					if r == models.RoleRegularUser {
						go func() {
							_ = h.profileSvc.RecordView(c.Request.Context(), userID, id)
						}()
						break
					}
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"offering": offering})
}

type offeringRequest struct {
	CategoryID      uint64  `json:"category_id"       binding:"required"`
	Name            string  `json:"name"              binding:"required,min=1,max=200"`
	Description     string  `json:"description"`
	BasePrice       float64 `json:"base_price"`
	DurationMinutes int     `json:"duration_minutes"  binding:"required,min=1"`
}

func (h *Handler) CreateOffering(c *gin.Context) {
	var req offeringRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.BadRequest(c, "validation_error", err.Error())
		return
	}

	agentID := c.GetUint64(auth.CtxUserID)
	offering, err := h.svc.CreateOffering(c.Request.Context(), agentID, CreateOfferingInput{
		CategoryID: req.CategoryID, Name: req.Name, Description: req.Description,
		BasePrice: req.BasePrice, DurationMinutes: req.DurationMinutes,
	})
	if err != nil {
		apierr.UnprocessableEntity(c, "validation_error", err.Error(), nil)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"offering": offering})
}

func (h *Handler) UpdateOffering(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		apierr.BadRequest(c, "invalid_param", "id must be a positive integer")
		return
	}
	var req offeringRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.BadRequest(c, "validation_error", err.Error())
		return
	}

	callerID := c.GetUint64(auth.CtxUserID)
	callerRoles := getRoles(c)

	offering, err := h.svc.UpdateOffering(c.Request.Context(), id, callerID, callerRoles, CreateOfferingInput{
		CategoryID: req.CategoryID, Name: req.Name, Description: req.Description,
		BasePrice: req.BasePrice, DurationMinutes: req.DurationMinutes,
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			apierr.NotFound(c, "offering")
			return
		}
		if errors.Is(err, ErrForbidden) {
			apierr.Forbidden(c)
			return
		}
		apierr.UnprocessableEntity(c, "validation_error", err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"offering": offering})
}

type statusRequest struct {
	Active bool `json:"active"`
}

func (h *Handler) ToggleStatus(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		apierr.BadRequest(c, "invalid_param", "id must be a positive integer")
		return
	}
	var req statusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.BadRequest(c, "validation_error", err.Error())
		return
	}

	callerID := c.GetUint64(auth.CtxUserID)
	callerRoles := getRoles(c)

	offering, err := h.svc.ToggleStatus(c.Request.Context(), id, callerID, callerRoles, req.Active)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			apierr.NotFound(c, "offering")
			return
		}
		if errors.Is(err, ErrForbidden) {
			apierr.Forbidden(c)
			return
		}
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"offering": offering})
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func parseID(c *gin.Context) (uint64, error) {
	return strconv.ParseUint(c.Param("id"), 10, 64)
}

func parseQueryUint64(c *gin.Context, name string) uint64 {
	v, _ := strconv.ParseUint(c.Query(name), 10, 64)
	return v
}

func parseQueryInt(c *gin.Context, name string, def int) int {
	v, err := strconv.Atoi(c.Query(name))
	if err != nil || v <= 0 {
		return def
	}
	return v
}

func getRoles(c *gin.Context) []string {
	if v, ok := c.Get(auth.CtxRoles); ok {
		if roles, ok := v.([]string); ok {
			return roles
		}
	}
	return nil
}
