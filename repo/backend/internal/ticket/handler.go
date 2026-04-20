package ticket

import (
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/eagle-point/service-portal/internal/apierr"
	"github.com/eagle-point/service-portal/internal/auth"
	"github.com/eagle-point/service-portal/internal/models"
	"github.com/eagle-point/service-portal/internal/upload"
)

// Attachment limits.
const (
	MaxAttachmentsPerTicket = 5
	MaxAttachmentBytes      = 5 * 1024 * 1024 // 5 MB
)

var allowedMimeTypes = map[string]string{
	"image/jpeg":      ".jpg",
	"image/png":       ".png",
	"application/pdf": ".pdf",
}

// Handler exposes ticket HTTP endpoints.
type Handler struct {
	svc *Service
}

// NewHandler creates a Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// ─── List / Get ──────────────────────────────────────────────────────────────

func (h *Handler) List(c *gin.Context) {
	userID := c.GetUint64(auth.CtxUserID)
	roles := getRoles(c)

	filter := ListFilter{Status: c.Query("status")}
	tickets, err := h.svc.List(c.Request.Context(), userID, roles, filter)
	if err != nil {
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"tickets": tickets})
}

func (h *Handler) Get(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		apierr.BadRequest(c, "invalid_param", "id must be a positive integer")
		return
	}
	t, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			apierr.NotFound(c, "ticket")
			return
		}
		apierr.InternalError(c)
		return
	}

	callerID := c.GetUint64(auth.CtxUserID)
	roles := getRoles(c)
	if !CanView(callerID, roles, t) {
		apierr.Forbidden(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ticket": t})
}

// ─── Create (multipart or JSON) ──────────────────────────────────────────────

func (h *Handler) Create(c *gin.Context) {
	contentType := c.ContentType()
	var (
		offeringID, categoryID, addressID uint64
		preferredStart, preferredEnd      time.Time
		deliveryMethod                    string
		shippingFee                       float64
		files                             []*multipart.FileHeader
	)

	if strings.HasPrefix(contentType, "multipart/") {
		form, err := c.MultipartForm()
		if err != nil {
			apierr.BadRequest(c, "invalid_body", "invalid multipart form")
			return
		}
		offeringID, _ = strconv.ParseUint(c.PostForm("offering_id"), 10, 64)
		categoryID, _ = strconv.ParseUint(c.PostForm("category_id"), 10, 64)
		addressID, _ = strconv.ParseUint(c.PostForm("address_id"), 10, 64)
		preferredStart, _ = parseTimeForm(c.PostForm("preferred_start"))
		preferredEnd, _ = parseTimeForm(c.PostForm("preferred_end"))
		deliveryMethod = c.PostForm("delivery_method")
		shippingFee, _ = strconv.ParseFloat(c.PostForm("shipping_fee"), 64)
		files = form.File["attachments"]
	} else {
		var body struct {
			OfferingID     uint64  `json:"offering_id"     binding:"required"`
			CategoryID     uint64  `json:"category_id"     binding:"required"`
			AddressID      uint64  `json:"address_id"      binding:"required"`
			PreferredStart string  `json:"preferred_start" binding:"required"`
			PreferredEnd   string  `json:"preferred_end"   binding:"required"`
			DeliveryMethod string  `json:"delivery_method"`
			ShippingFee    float64 `json:"shipping_fee"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			apierr.BadRequest(c, "validation_error", err.Error())
			return
		}
		offeringID = body.OfferingID
		categoryID = body.CategoryID
		addressID = body.AddressID
		preferredStart, _ = parseTimeForm(body.PreferredStart)
		preferredEnd, _ = parseTimeForm(body.PreferredEnd)
		deliveryMethod = body.DeliveryMethod
		shippingFee = body.ShippingFee
	}

	if len(files) > MaxAttachmentsPerTicket {
		apierr.UnprocessableEntity(c, "too_many_files",
			fmt.Sprintf("at most %d attachments allowed", MaxAttachmentsPerTicket), nil)
		return
	}
	// Declared-size fast rejection happens here; full MIME + magic-byte
	// checks run inside upload.Save after the ticket row is created.
	for _, f := range files {
		if f.Size > MaxAttachmentBytes {
			apierr.UnprocessableEntity(c, "file_too_large",
				fmt.Sprintf("%s exceeds %d bytes", f.Filename, MaxAttachmentBytes), nil)
			return
		}
	}

	userID := c.GetUint64(auth.CtxUserID)
	t, err := h.svc.Create(c.Request.Context(), CreateInput{
		UserID:         userID,
		OfferingID:     offeringID,
		CategoryID:     categoryID,
		AddressID:      addressID,
		PreferredStart: preferredStart,
		PreferredEnd:   preferredEnd,
		DeliveryMethod: deliveryMethod,
		ShippingFee:    shippingFee,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrForbidden):
			apierr.Forbidden(c)
		case errors.Is(err, ErrValidation):
			apierr.UnprocessableEntity(c, "validation_error", err.Error(), nil)
		default:
			apierr.InternalError(c)
		}
		return
	}

	// Save attachments via the hardened upload helper. Detected MIME +
	// sanitized filename are the authoritative values written to the DB —
	// the client-declared Content-Type is never persisted.
	rules := upload.Rules{MaxBytes: MaxAttachmentBytes, AllowedMIMEs: allowedMimeTypes}
	dir := filepath.Join(h.svc.storageDir, "tickets", strconv.FormatUint(t.ID, 10))
	for _, f := range files {
		res, err := upload.Save(rules, f, dir)
		if err != nil {
			apierr.UnprocessableEntity(c, "invalid_file", err.Error(), nil)
			return
		}
		if _, err := h.svc.RecordAttachment(c.Request.Context(),
			t.ID, res.StoredFilename, res.SanitizedOriginal, res.DetectedMIME,
			uint64(res.SizeBytes), res.StoragePath); err != nil {
			apierr.InternalError(c)
			return
		}
	}

	// Reload to return the complete object (no attachments embedded yet, but the caller can list)
	c.JSON(http.StatusCreated, gin.H{"ticket": t})
}

// ─── Status transition ───────────────────────────────────────────────────────

type statusRequest struct {
	Status       string `json:"status" binding:"required"`
	CancelReason string `json:"cancel_reason"`
}

func (h *Handler) UpdateStatus(c *gin.Context) {
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
	roles := getRoles(c)

	t, err := h.svc.UpdateStatus(c.Request.Context(), id, callerID, roles, req.Status, req.CancelReason)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			apierr.NotFound(c, "ticket")
		case errors.Is(err, ErrForbidden):
			apierr.Forbidden(c)
		case errors.Is(err, ErrInvalidTransition):
			apierr.UnprocessableEntity(c, "invalid_transition", err.Error(), nil)
		default:
			apierr.InternalError(c)
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"ticket": t})
}

// ─── Notes ───────────────────────────────────────────────────────────────────

type noteRequest struct {
	Content string `json:"content" binding:"required"`
}

func (h *Handler) ListNotes(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		apierr.BadRequest(c, "invalid_param", "id must be a positive integer")
		return
	}
	t, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			apierr.NotFound(c, "ticket")
			return
		}
		apierr.InternalError(c)
		return
	}
	if !CanView(c.GetUint64(auth.CtxUserID), getRoles(c), t) {
		apierr.Forbidden(c)
		return
	}

	notes, err := h.svc.ListNotes(c.Request.Context(), id)
	if err != nil {
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"notes": notes})
}

func (h *Handler) CreateNote(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		apierr.BadRequest(c, "invalid_param", "id must be a positive integer")
		return
	}
	var req noteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.BadRequest(c, "validation_error", err.Error())
		return
	}

	t, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			apierr.NotFound(c, "ticket")
			return
		}
		apierr.InternalError(c)
		return
	}
	callerID := c.GetUint64(auth.CtxUserID)
	if !CanView(callerID, getRoles(c), t) {
		apierr.Forbidden(c)
		return
	}

	n, err := h.svc.CreateNote(c.Request.Context(), id, callerID, req.Content)
	if err != nil {
		if errors.Is(err, ErrValidation) {
			apierr.UnprocessableEntity(c, "validation_error", err.Error(), nil)
			return
		}
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"note": n})
}

// ─── Attachments ─────────────────────────────────────────────────────────────

func (h *Handler) ListAttachments(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		apierr.BadRequest(c, "invalid_param", "id must be a positive integer")
		return
	}
	t, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			apierr.NotFound(c, "ticket")
			return
		}
		apierr.InternalError(c)
		return
	}
	if !CanView(c.GetUint64(auth.CtxUserID), getRoles(c), t) {
		apierr.Forbidden(c)
		return
	}
	att, err := h.svc.ListAttachments(c.Request.Context(), id)
	if err != nil {
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"attachments": att})
}

func (h *Handler) DeleteAttachment(c *gin.Context) {
	ticketID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || ticketID == 0 {
		apierr.BadRequest(c, "invalid_param", "id must be a positive integer")
		return
	}
	fileID, err := strconv.ParseUint(c.Param("file_id"), 10, 64)
	if err != nil || fileID == 0 {
		apierr.BadRequest(c, "invalid_param", "file_id must be a positive integer")
		return
	}

	a, err := h.svc.GetAttachment(c.Request.Context(), fileID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			apierr.NotFound(c, "attachment")
			return
		}
		apierr.InternalError(c)
		return
	}
	if a.TicketID != ticketID {
		apierr.NotFound(c, "attachment")
		return
	}

	t, err := h.svc.Get(c.Request.Context(), ticketID)
	if err != nil {
		apierr.InternalError(c)
		return
	}
	callerID := c.GetUint64(auth.CtxUserID)
	roles := getRoles(c)
	if !hasRole(roles, models.RoleAdministrator) && t.UserID != callerID {
		apierr.Forbidden(c)
		return
	}

	if err := h.svc.DeleteAttachment(c.Request.Context(), fileID); err != nil {
		apierr.InternalError(c)
		return
	}
	// Best-effort disk cleanup
	_ = os.Remove(a.StoragePath)
	c.Status(http.StatusNoContent)
}

// ─── misc helpers ────────────────────────────────────────────────────────────

func parseID(c *gin.Context) (uint64, error) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		return 0, errors.New("bad id")
	}
	return id, nil
}

func getRoles(c *gin.Context) []string {
	if v, ok := c.Get(auth.CtxRoles); ok {
		if roles, ok := v.([]string); ok {
			return roles
		}
	}
	return nil
}

// parseTimeForm accepts RFC3339 and datetime-local formats.
func parseTimeForm(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, errors.New("empty time")
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02T15:04", s)
}
