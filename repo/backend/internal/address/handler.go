package address

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/eagle-point/service-portal/internal/apierr"
	"github.com/eagle-point/service-portal/internal/auth"
)

// Handler exposes address-book CRUD over HTTP.
type Handler struct {
	svc *Service
}

// NewHandler creates a Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// ─── GET /api/v1/users/me/addresses ──────────────────────────────────────────

func (h *Handler) List(c *gin.Context) {
	userID := c.GetUint64(auth.CtxUserID)
	addrs, err := h.svc.List(c.Request.Context(), userID)
	if err != nil {
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"addresses": addrs})
}

// ─── POST /api/v1/users/me/addresses ─────────────────────────────────────────

type createRequest struct {
	Label        string `json:"label"`
	AddressLine1 string `json:"address_line1" binding:"required"`
	AddressLine2 string `json:"address_line2"`
	City         string `json:"city"  binding:"required"`
	State        string `json:"state" binding:"required,len=2"`
	Zip          string `json:"zip"   binding:"required"`
}

func (h *Handler) Create(c *gin.Context) {
	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.BadRequest(c, "validation_error", err.Error())
		return
	}

	userID := c.GetUint64(auth.CtxUserID)
	addr, err := h.svc.Create(c.Request.Context(), userID, CreateInput{
		Label:        req.Label,
		AddressLine1: req.AddressLine1,
		AddressLine2: req.AddressLine2,
		City:         req.City,
		State:        req.State,
		Zip:          req.Zip,
	})
	if err != nil {
		apierr.UnprocessableEntity(c, "validation_error", err.Error(), nil)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"address": addr})
}

// ─── PUT /api/v1/users/me/addresses/:id ──────────────────────────────────────

func (h *Handler) Update(c *gin.Context) {
	addrID, err := parseID(c)
	if err != nil {
		apierr.BadRequest(c, "invalid_param", "id must be a positive integer")
		return
	}

	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.BadRequest(c, "validation_error", err.Error())
		return
	}

	userID := c.GetUint64(auth.CtxUserID)
	addr, err := h.svc.Update(c.Request.Context(), userID, addrID, UpdateInput{
		Label:        req.Label,
		AddressLine1: req.AddressLine1,
		AddressLine2: req.AddressLine2,
		City:         req.City,
		State:        req.State,
		Zip:          req.Zip,
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			apierr.NotFound(c, "address")
			return
		}
		apierr.UnprocessableEntity(c, "validation_error", err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"address": addr})
}

// ─── DELETE /api/v1/users/me/addresses/:id ───────────────────────────────────

func (h *Handler) Delete(c *gin.Context) {
	addrID, err := parseID(c)
	if err != nil {
		apierr.BadRequest(c, "invalid_param", "id must be a positive integer")
		return
	}

	userID := c.GetUint64(auth.CtxUserID)
	if err := h.svc.Delete(c.Request.Context(), userID, addrID); err != nil {
		if errors.Is(err, ErrNotFound) {
			apierr.NotFound(c, "address")
			return
		}
		apierr.InternalError(c)
		return
	}
	c.Status(http.StatusNoContent)
}

// ─── PUT /api/v1/users/me/addresses/:id/default ──────────────────────────────

func (h *Handler) SetDefault(c *gin.Context) {
	addrID, err := parseID(c)
	if err != nil {
		apierr.BadRequest(c, "invalid_param", "id must be a positive integer")
		return
	}

	userID := c.GetUint64(auth.CtxUserID)
	addr, err := h.svc.SetDefault(c.Request.Context(), userID, addrID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			apierr.NotFound(c, "address")
			return
		}
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"address": addr})
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func parseID(c *gin.Context) (uint64, error) {
	return strconv.ParseUint(c.Param("id"), 10, 64)
}
