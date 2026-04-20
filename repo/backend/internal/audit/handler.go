package audit

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/eagle-point/service-portal/internal/apierr"
)

// Handler exposes admin-only audit log read endpoints.
type Handler struct {
	svc *Service
}

// NewHandler creates a Handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// AdminList returns recent audit entries (filterable by user_id).
func (h *Handler) AdminList(c *gin.Context) {
	userID, _ := strconv.ParseUint(c.Query("user_id"), 10, 64)
	limit, _ := strconv.Atoi(c.Query("limit"))
	items, err := h.svc.List(c.Request.Context(), userID, limit)
	if err != nil {
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"audit_logs": items})
}
