package hmacadmin

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/eagle-point/service-portal/internal/apierr"
	"github.com/eagle-point/service-portal/internal/audit"
	"github.com/eagle-point/service-portal/internal/auth"
	"github.com/eagle-point/service-portal/internal/models"
)

// Handler exposes HMAC key management over HTTP. All routes must be mounted
// behind admin-only RBAC — the handler trusts that authorization is enforced
// upstream.
type Handler struct {
	svc   *Service
	audit *audit.Service
}

// NewHandler wires a Handler. If auditSvc is nil the handler still works but
// emits no audit log entries (useful for tests).
func NewHandler(svc *Service, auditSvc *audit.Service) *Handler {
	return &Handler{svc: svc, audit: auditSvc}
}

// ─── GET /api/v1/admin/hmac-keys ─────────────────────────────────────────────

func (h *Handler) List(c *gin.Context) {
	keys, err := h.svc.List(c.Request.Context())
	if err != nil {
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"keys": keys})
}

// ─── POST /api/v1/admin/hmac-keys ────────────────────────────────────────────

type createRequest struct {
	KeyID string `json:"key_id" binding:"required"`
}

func (h *Handler) Create(c *gin.Context) {
	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.BadRequest(c, "validation_error", err.Error())
		return
	}

	reveal, err := h.svc.Create(c.Request.Context(), req.KeyID)
	if err != nil {
		h.respondServiceErr(c, err)
		return
	}

	h.writeAudit(c, "hmac_key_create", reveal.ID, reveal.KeyID)

	// 201 Created with one-shot secret body. The secret field is the only
	// chance the admin has to copy this value — make the response self-evident.
	c.JSON(http.StatusCreated, gin.H{
		"key":                   reveal.KeyInfo,
		"secret":                reveal.Secret,
		"secret_reveal_warning": "Copy this secret now. It will NOT be shown again.",
	})
}

// ─── POST /api/v1/admin/hmac-keys/rotate ─────────────────────────────────────

type rotateRequest struct {
	KeyID string `json:"key_id" binding:"required"`
}

func (h *Handler) Rotate(c *gin.Context) {
	var req rotateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.BadRequest(c, "validation_error", err.Error())
		return
	}

	reveal, err := h.svc.Rotate(c.Request.Context(), req.KeyID)
	if err != nil {
		h.respondServiceErr(c, err)
		return
	}

	h.writeAudit(c, "hmac_key_rotate", reveal.ID, reveal.KeyID)

	c.JSON(http.StatusOK, gin.H{
		"key":                   reveal.KeyInfo,
		"secret":                reveal.Secret,
		"secret_reveal_warning": "Copy this secret now. It will NOT be shown again.",
	})
}

// ─── DELETE /api/v1/admin/hmac-keys/:id ──────────────────────────────────────

func (h *Handler) Revoke(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 0 {
		apierr.BadRequest(c, "validation_error", "id must be a positive integer")
		return
	}

	info, err := h.svc.Revoke(c.Request.Context(), id)
	if err != nil {
		h.respondServiceErr(c, err)
		return
	}

	h.writeAudit(c, "hmac_key_revoke", info.ID, info.KeyID)

	c.JSON(http.StatusOK, gin.H{"key": info})
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// respondServiceErr translates sentinel errors to appropriate HTTP responses.
func (h *Handler) respondServiceErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrKeyIDRequired), errors.Is(err, ErrKeyIDInvalid):
		apierr.BadRequest(c, "validation_error", err.Error())
	case errors.Is(err, ErrKeyIDExists):
		apierr.Conflict(c, "key_exists", err.Error())
	case errors.Is(err, ErrKeyNotFound):
		apierr.NotFound(c, "hmac key")
	default:
		apierr.InternalError(c)
	}
}

// writeAudit records an admin_operation entry for an HMAC key lifecycle event.
// Silently no-ops when the Handler was built without an audit service.
// Errors from the audit writer are swallowed: audit failure must not mask the
// caller's successful business response.
func (h *Handler) writeAudit(c *gin.Context, op string, entityID uint64, keyID string) {
	if h.audit == nil {
		return
	}
	uid := c.GetUint64(auth.CtxUserID)
	var uidPtr *uint64
	if uid > 0 {
		uidPtr = &uid
	}
	eid := entityID
	_ = h.audit.Write(c.Request.Context(), audit.Entry{
		UserID:     uidPtr,
		Action:     models.AuditActionAdminOp,
		EntityType: "hmac_key",
		EntityID:   &eid,
		IPAddress:  c.ClientIP(),
		UserAgent:  c.GetHeader("User-Agent"),
		Metadata: map[string]any{
			"op":     op,
			"key_id": keyID,
		},
	})
}
