package profile

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/eagle-point/service-portal/internal/apierr"
	"github.com/eagle-point/service-portal/internal/auth"
	"github.com/eagle-point/service-portal/internal/models"
)

// Handler exposes profile, preferences, favorites and history over HTTP.
type Handler struct {
	svc *Service
}

// NewHandler creates a Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// ─── GET /api/v1/users/me/profile ────────────────────────────────────────────

func (h *Handler) GetProfile(c *gin.Context) {
	userID := c.GetUint64(auth.CtxUserID)
	roles, _ := c.Get(auth.CtxRoles)
	isAdmin := hasRole(roles, models.RoleAdministrator)

	p, err := h.svc.GetProfile(c.Request.Context(), userID, isAdmin)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			apierr.NotFound(c, "user")
			return
		}
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"profile": p})
}

// ─── PUT /api/v1/users/me/profile ────────────────────────────────────────────

type updateProfileRequest struct {
	DisplayName string `json:"display_name" binding:"required,min=1,max=100"`
	AvatarURL   string `json:"avatar_url"`
	Bio         string `json:"bio"`
	Phone       string `json:"phone"`
}

func (h *Handler) UpdateProfile(c *gin.Context) {
	var req updateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.BadRequest(c, "validation_error", err.Error())
		return
	}

	userID := c.GetUint64(auth.CtxUserID)
	p, err := h.svc.UpdateProfile(c.Request.Context(), userID, UpdateProfileInput{
		DisplayName: req.DisplayName,
		AvatarURL:   req.AvatarURL,
		Bio:         req.Bio,
		Phone:       req.Phone,
	})
	if err != nil {
		apierr.UnprocessableEntity(c, "validation_error", err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"profile": p})
}

// ─── GET /api/v1/users/me/preferences ────────────────────────────────────────

func (h *Handler) GetPreferences(c *gin.Context) {
	userID := c.GetUint64(auth.CtxUserID)
	prefs, err := h.svc.GetPreferences(c.Request.Context(), userID)
	if err != nil {
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"preferences": prefs})
}

// ─── PUT /api/v1/users/me/preferences ────────────────────────────────────────

type updatePreferencesRequest struct {
	NotifyInApp  bool    `json:"notify_in_app"`
	MutedTags    []int64 `json:"muted_tags"`
	MutedAuthors []int64 `json:"muted_authors"`
}

func (h *Handler) UpdatePreferences(c *gin.Context) {
	var req updatePreferencesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.BadRequest(c, "validation_error", err.Error())
		return
	}

	userID := c.GetUint64(auth.CtxUserID)
	prefs, err := h.svc.UpdatePreferences(c.Request.Context(), userID, UpdatePreferencesInput{
		NotifyInApp:  req.NotifyInApp,
		MutedTags:    req.MutedTags,
		MutedAuthors: req.MutedAuthors,
	})
	if err != nil {
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"preferences": prefs})
}

// ─── GET /api/v1/users/me/favorites ──────────────────────────────────────────

func (h *Handler) ListFavorites(c *gin.Context) {
	userID := c.GetUint64(auth.CtxUserID)
	cursor := parseUint64Param(c, "cursor")
	limit := parseIntParam(c, "limit", 20)

	page, err := h.svc.ListFavorites(c.Request.Context(), userID, cursor, limit)
	if err != nil {
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, page)
}

// ─── POST /api/v1/users/me/favorites ─────────────────────────────────────────

type addFavoriteRequest struct {
	OfferingID uint64 `json:"offering_id" binding:"required"`
}

func (h *Handler) AddFavorite(c *gin.Context) {
	var req addFavoriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.BadRequest(c, "validation_error", err.Error())
		return
	}

	userID := c.GetUint64(auth.CtxUserID)
	if err := h.svc.AddFavorite(c.Request.Context(), userID, req.OfferingID); err != nil {
		apierr.InternalError(c)
		return
	}
	c.Status(http.StatusNoContent)
}

// ─── DELETE /api/v1/users/me/favorites/:offering_id ──────────────────────────

func (h *Handler) RemoveFavorite(c *gin.Context) {
	userID := c.GetUint64(auth.CtxUserID)
	offeringID, err := strconv.ParseUint(c.Param("offering_id"), 10, 64)
	if err != nil || offeringID == 0 {
		apierr.BadRequest(c, "invalid_param", "offering_id must be a positive integer")
		return
	}

	if err := h.svc.RemoveFavorite(c.Request.Context(), userID, offeringID); err != nil {
		apierr.InternalError(c)
		return
	}
	c.Status(http.StatusNoContent)
}

// ─── GET /api/v1/users/me/history ────────────────────────────────────────────

func (h *Handler) ListHistory(c *gin.Context) {
	userID := c.GetUint64(auth.CtxUserID)
	cursor := parseUint64Param(c, "cursor")
	limit := parseIntParam(c, "limit", 20)

	page, err := h.svc.ListHistory(c.Request.Context(), userID, cursor, limit)
	if err != nil {
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, page)
}

// ─── DELETE /api/v1/users/me/history ─────────────────────────────────────────

func (h *Handler) ClearHistory(c *gin.Context) {
	userID := c.GetUint64(auth.CtxUserID)
	if err := h.svc.ClearHistory(c.Request.Context(), userID); err != nil {
		apierr.InternalError(c)
		return
	}
	c.Status(http.StatusNoContent)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func hasRole(rolesVal interface{}, target string) bool {
	roles, ok := rolesVal.([]string)
	if !ok {
		return false
	}
	for _, r := range roles {
		if r == target {
			return true
		}
	}
	return false
}

func parseUint64Param(c *gin.Context, name string) uint64 {
	v, _ := strconv.ParseUint(c.Query(name), 10, 64)
	return v
}

func parseIntParam(c *gin.Context, name string, def int) int {
	v, err := strconv.Atoi(c.Query(name))
	if err != nil || v <= 0 {
		return def
	}
	return v
}
