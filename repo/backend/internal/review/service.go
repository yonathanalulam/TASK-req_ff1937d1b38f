package review

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/eagle-point/service-portal/internal/models"
)

// Errors.
var (
	ErrNotFound       = errors.New("not found")
	ErrForbidden      = errors.New("forbidden")
	ErrNotEligible    = errors.New("ticket not eligible for review")
	ErrAlreadyExists  = errors.New("review already exists for this ticket")
	ErrValidation     = errors.New("validation error")
)

// Service provides review + report business logic.
type Service struct {
	db         *sql.DB
	storageDir string
}

// NewService creates a Service.
func NewService(db *sql.DB, storageDir string) *Service {
	if storageDir == "" {
		storageDir = "storage/uploads"
	}
	return &Service{db: db, storageDir: storageDir}
}

// StorageDir returns the base upload directory.
func (s *Service) StorageDir() string { return s.storageDir }

// ─── Inputs ──────────────────────────────────────────────────────────────────

// CreateInput for a new review.
type CreateInput struct {
	TicketID uint64
	UserID   uint64
	Rating   int
	Text     string
}

// UpdateInput for editing a review.
type UpdateInput struct {
	Rating int
	Text   string
}

// ─── CRUD ────────────────────────────────────────────────────────────────────

// Create inserts a new review. Enforces:
//   - Ticket exists, is owned by the user, and is in Completed/Closed status
//   - Only one review per ticket (UNIQUE constraint)
func (s *Service) Create(ctx context.Context, in CreateInput) (*models.Review, error) {
	if in.Rating < 1 || in.Rating > 5 {
		return nil, fmt.Errorf("%w: rating must be 1-5", ErrValidation)
	}

	var (
		ticketUserID uint64
		offeringID   uint64
		status       string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id, offering_id, status FROM tickets WHERE id = ?`, in.TicketID,
	).Scan(&ticketUserID, &offeringID, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("review.Create: load ticket: %w", err)
	}
	if ticketUserID != in.UserID {
		return nil, ErrForbidden
	}
	if status != models.TicketStatusCompleted && status != models.TicketStatusClosed {
		return nil, ErrNotEligible
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO reviews (ticket_id, user_id, offering_id, rating, text, status)
		 VALUES (?, ?, ?, ?, ?, 'published')`,
		in.TicketID, in.UserID, offeringID, in.Rating, strings.TrimSpace(in.Text),
	)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return nil, ErrAlreadyExists
		}
		return nil, fmt.Errorf("review.Create: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.Get(ctx, uint64(id))
}

// Get returns a review by ID including images.
func (s *Service) Get(ctx context.Context, id uint64) (*models.Review, error) {
	r := &models.Review{}
	var text sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, ticket_id, user_id, offering_id, rating, text, status, created_at, updated_at
		 FROM reviews WHERE id = ?`, id,
	).Scan(&r.ID, &r.TicketID, &r.UserID, &r.OfferingID, &r.Rating,
		&text, &r.Status, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if text.Valid {
		r.Text = text.String
	}

	imgs, err := s.loadImages(ctx, r.ID)
	if err == nil {
		r.Images = imgs
	}
	return r, nil
}

// ListByOffering returns published reviews for an offering, paginated.
func (s *Service) ListByOffering(ctx context.Context, offeringID uint64, cursor uint64, limit int) ([]*models.Review, uint64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, ticket_id, user_id, offering_id, rating, text, status, created_at, updated_at
		 FROM reviews
		 WHERE offering_id = ? AND status = 'published'
		   AND (? = 0 OR id < ?)
		 ORDER BY id DESC
		 LIMIT ?`,
		offeringID, cursor, cursor, limit+1,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("review.ListByOffering: %w", err)
	}
	defer rows.Close()

	var all []*models.Review
	for rows.Next() {
		r := &models.Review{}
		var text sql.NullString
		if err := rows.Scan(&r.ID, &r.TicketID, &r.UserID, &r.OfferingID, &r.Rating,
			&text, &r.Status, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, 0, err
		}
		if text.Valid {
			r.Text = text.String
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	var next uint64
	if len(all) > limit {
		next = all[limit-1].ID
		all = all[:limit]
	}
	if all == nil {
		all = []*models.Review{}
	}

	// Batch-load images per review
	for _, r := range all {
		if imgs, err := s.loadImages(ctx, r.ID); err == nil {
			r.Images = imgs
		}
	}
	return all, next, nil
}

// Update edits an existing review (owner only; enforced at handler).
func (s *Service) Update(ctx context.Context, id, callerID uint64, in UpdateInput) (*models.Review, error) {
	r, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if r.UserID != callerID {
		return nil, ErrForbidden
	}
	if in.Rating < 1 || in.Rating > 5 {
		return nil, fmt.Errorf("%w: rating must be 1-5", ErrValidation)
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE reviews SET rating = ?, text = ? WHERE id = ?`,
		in.Rating, strings.TrimSpace(in.Text), id)
	if err != nil {
		return nil, fmt.Errorf("review.Update: %w", err)
	}
	return s.Get(ctx, id)
}

// ─── Summary ─────────────────────────────────────────────────────────────────

// Summary returns aggregated metrics for an offering.
func (s *Service) Summary(ctx context.Context, offeringID uint64) (*models.ReviewSummary, error) {
	var (
		total    int
		avg      sql.NullFloat64
		positive int
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*), AVG(rating), SUM(CASE WHEN rating >= 4 THEN 1 ELSE 0 END)
		 FROM reviews WHERE offering_id = ? AND status = 'published'`,
		offeringID,
	).Scan(&total, &avg, &positive)
	if err != nil {
		return nil, fmt.Errorf("review.Summary: %w", err)
	}

	sum := &models.ReviewSummary{TotalReviews: total}
	if avg.Valid {
		sum.AverageRating = avg.Float64
	}
	if total > 0 {
		sum.PositiveRate = float64(positive) / float64(total)
	}
	return sum, nil
}

// ─── Images ──────────────────────────────────────────────────────────────────

// RecordImage inserts a review image row.
func (s *Service) RecordImage(ctx context.Context, reviewID uint64, filename, storagePath string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO review_images (review_id, filename, storage_path) VALUES (?, ?, ?)`,
		reviewID, filename, storagePath)
	return err
}

func (s *Service) loadImages(ctx context.Context, reviewID uint64) ([]models.ReviewImage, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, review_id, filename, storage_path, created_at
		 FROM review_images WHERE review_id = ? ORDER BY id ASC`, reviewID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ReviewImage
	for rows.Next() {
		var im models.ReviewImage
		if err := rows.Scan(&im.ID, &im.ReviewID, &im.Filename, &im.StoragePath, &im.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, im)
	}
	return out, rows.Err()
}

// ─── Reports ─────────────────────────────────────────────────────────────────

var validReportReasons = map[string]struct{}{
	"spam":       {},
	"abusive":    {},
	"irrelevant": {},
}

// CreateReport records an abuse report against a review.
func (s *Service) CreateReport(ctx context.Context, reviewID, reporterID uint64, reason, details string) (*models.ReviewReport, error) {
	if _, ok := validReportReasons[reason]; !ok {
		return nil, fmt.Errorf("%w: reason must be spam, abusive, or irrelevant", ErrValidation)
	}
	// Verify review exists
	if _, err := s.Get(ctx, reviewID); err != nil {
		return nil, err
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO review_reports (review_id, reporter_id, reason, details)
		 VALUES (?, ?, ?, ?)`,
		reviewID, reporterID, reason, strings.TrimSpace(details),
	)
	if err != nil {
		return nil, fmt.Errorf("review.CreateReport: %w", err)
	}
	id, _ := res.LastInsertId()
	r := &models.ReviewReport{}
	var det sql.NullString
	err = s.db.QueryRowContext(ctx,
		`SELECT id, review_id, reporter_id, reason, details, created_at
		 FROM review_reports WHERE id = ?`, id,
	).Scan(&r.ID, &r.ReviewID, &r.ReporterID, &r.Reason, &det, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	if det.Valid {
		r.Details = det.String
	}
	return r, nil
}
