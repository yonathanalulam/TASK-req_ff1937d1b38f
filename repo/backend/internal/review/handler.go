package review

import (
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/eagle-point/service-portal/internal/apierr"
	"github.com/eagle-point/service-portal/internal/auth"
	"github.com/eagle-point/service-portal/internal/upload"
)

// Limits.
const (
	MaxReviewImages    = 3
	MaxReviewImageSize = 5 * 1024 * 1024 // 5 MB
)

var allowedImageMimes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
}

// BorderlineHook is invoked after a successful create when the screening
// middleware flagged borderline terms. Wired by the router so the review
// package does not depend on the moderation package.
type BorderlineHook func(ctx context.Context, contentType string, contentID uint64, text string, flagged []string) error

// Context keys mirrored from moderation.middleware (avoids cross-package import).
const (
	borderlineTextKey  = "ctx_borderline_text"
	borderlineTermsKey = "ctx_borderline_terms"
)

// Handler exposes review HTTP endpoints.
type Handler struct {
	svc          *Service
	onBorderline BorderlineHook
}

// NewHandler creates a Handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// SetBorderlineHook wires the optional moderation enqueue hook.
func (h *Handler) SetBorderlineHook(fn BorderlineHook) { h.onBorderline = fn }

// ─── Create review ───────────────────────────────────────────────────────────

func (h *Handler) Create(c *gin.Context) {
	ticketID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || ticketID == 0 {
		apierr.BadRequest(c, "invalid_param", "id must be a positive integer")
		return
	}

	var (
		rating int
		text   string
		files  []*multipart.FileHeader
	)

	if strings.HasPrefix(c.ContentType(), "multipart/") {
		form, err := c.MultipartForm()
		if err != nil {
			apierr.BadRequest(c, "invalid_body", "invalid multipart form")
			return
		}
		rating, _ = strconv.Atoi(c.PostForm("rating"))
		text = c.PostForm("text")
		files = form.File["images"]
	} else {
		var body struct {
			Rating int    `json:"rating" binding:"required"`
			Text   string `json:"text"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			apierr.BadRequest(c, "validation_error", err.Error())
			return
		}
		rating = body.Rating
		text = body.Text
	}

	if len(files) > MaxReviewImages {
		apierr.UnprocessableEntity(c, "too_many_files",
			fmt.Sprintf("at most %d images allowed", MaxReviewImages), nil)
		return
	}
	// Declared-size fast rejection only; the full MIME / magic-byte gate
	// runs inside upload.Save once the review row is created.
	for _, f := range files {
		if f.Size > MaxReviewImageSize {
			apierr.UnprocessableEntity(c, "file_too_large",
				fmt.Sprintf("%s exceeds 5MB", f.Filename), nil)
			return
		}
	}

	userID := c.GetUint64(auth.CtxUserID)
	r, err := h.svc.Create(c.Request.Context(), CreateInput{
		TicketID: ticketID,
		UserID:   userID,
		Rating:   rating,
		Text:     text,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			apierr.NotFound(c, "ticket")
		case errors.Is(err, ErrForbidden):
			apierr.Forbidden(c)
		case errors.Is(err, ErrNotEligible):
			apierr.UnprocessableEntity(c, "ticket_not_eligible",
				"review can only be posted for Completed or Closed tickets", nil)
		case errors.Is(err, ErrAlreadyExists):
			apierr.Conflict(c, "already_exists", "a review already exists for this ticket")
		case errors.Is(err, ErrValidation):
			apierr.UnprocessableEntity(c, "validation_error", err.Error(), nil)
		default:
			apierr.InternalError(c)
		}
		return
	}

	// Save images via the hardened upload helper.
	rules := upload.Rules{MaxBytes: MaxReviewImageSize, AllowedMIMEs: allowedImageMimes}
	dir := filepath.Join(h.svc.storageDir, "reviews", strconv.FormatUint(r.ID, 10))
	for _, f := range files {
		res, err := upload.Save(rules, f, dir)
		if err != nil {
			apierr.UnprocessableEntity(c, "invalid_file", err.Error(), nil)
			return
		}
		if err := h.svc.RecordImage(c.Request.Context(), r.ID, res.StoredFilename, res.StoragePath); err != nil {
			apierr.InternalError(c)
			return
		}
	}

	// If the screening middleware flagged borderline terms, demote to
	// pending_moderation and enqueue for review. Best-effort: a hook failure
	// must not undo the user's submission.
	if h.onBorderline != nil {
		if textVal, ok := c.Get(borderlineTextKey); ok {
			text, _ := textVal.(string)
			var terms []string
			if v, ok := c.Get(borderlineTermsKey); ok {
				terms, _ = v.([]string)
			}
			_ = h.onBorderline(c.Request.Context(), "review", r.ID, text, terms)
		}
	}

	// Reload to include images (and the updated status if borderline)
	full, _ := h.svc.Get(c.Request.Context(), r.ID)
	c.JSON(http.StatusCreated, gin.H{"review": full})
}

// ─── List by offering ────────────────────────────────────────────────────────

func (h *Handler) ListByOffering(c *gin.Context) {
	offeringID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || offeringID == 0 {
		apierr.BadRequest(c, "invalid_param", "id must be a positive integer")
		return
	}
	cursor, _ := strconv.ParseUint(c.Query("cursor"), 10, 64)
	limit, _ := strconv.Atoi(c.Query("limit"))

	items, next, err := h.svc.ListByOffering(c.Request.Context(), offeringID, cursor, limit)
	if err != nil {
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "next_cursor": next})
}

// ─── Summary ─────────────────────────────────────────────────────────────────

func (h *Handler) Summary(c *gin.Context) {
	offeringID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || offeringID == 0 {
		apierr.BadRequest(c, "invalid_param", "id must be a positive integer")
		return
	}
	sum, err := h.svc.Summary(c.Request.Context(), offeringID)
	if err != nil {
		apierr.InternalError(c)
		return
	}
	c.JSON(http.StatusOK, sum)
}

// ─── Update review ───────────────────────────────────────────────────────────

type updateRequest struct {
	Rating int    `json:"rating" binding:"required"`
	Text   string `json:"text"`
}

func (h *Handler) Update(c *gin.Context) {
	// Route: PUT /api/v1/tickets/:id/reviews/:review_id
	reviewID, err := strconv.ParseUint(c.Param("review_id"), 10, 64)
	if err != nil || reviewID == 0 {
		apierr.BadRequest(c, "invalid_param", "review_id must be a positive integer")
		return
	}
	var req updateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.BadRequest(c, "validation_error", err.Error())
		return
	}
	callerID := c.GetUint64(auth.CtxUserID)
	r, err := h.svc.Update(c.Request.Context(), reviewID, callerID, UpdateInput{
		Rating: req.Rating, Text: req.Text,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			apierr.NotFound(c, "review")
		case errors.Is(err, ErrForbidden):
			apierr.Forbidden(c)
		case errors.Is(err, ErrValidation):
			apierr.UnprocessableEntity(c, "validation_error", err.Error(), nil)
		default:
			apierr.InternalError(c)
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"review": r})
}

// ─── Report a review ─────────────────────────────────────────────────────────

type reportRequest struct {
	Reason  string `json:"reason" binding:"required"`
	Details string `json:"details"`
}

func (h *Handler) CreateReport(c *gin.Context) {
	reviewID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || reviewID == 0 {
		apierr.BadRequest(c, "invalid_param", "id must be a positive integer")
		return
	}
	var req reportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierr.BadRequest(c, "validation_error", err.Error())
		return
	}
	reporterID := c.GetUint64(auth.CtxUserID)
	rpt, err := h.svc.CreateReport(c.Request.Context(), reviewID, reporterID, req.Reason, req.Details)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			apierr.NotFound(c, "review")
		case errors.Is(err, ErrValidation):
			apierr.UnprocessableEntity(c, "validation_error", err.Error(), nil)
		default:
			apierr.InternalError(c)
		}
		return
	}
	c.JSON(http.StatusCreated, gin.H{"report": rpt})
}

// ─── Image upload helper ─────────────────────────────────────────────────────

