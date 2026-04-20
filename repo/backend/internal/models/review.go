package models

import "time"

// Review status constants.
const (
	ReviewStatusPublished        = "published"
	ReviewStatusPendingModeration = "pending_moderation"
	ReviewStatusRejected         = "rejected"
)

// Review represents a user's review of a completed ticket.
type Review struct {
	ID         uint64    `json:"id"`
	TicketID   uint64    `json:"ticket_id"`
	UserID     uint64    `json:"user_id"`
	OfferingID uint64    `json:"offering_id"`
	Rating     int       `json:"rating"`
	Text       string    `json:"text,omitempty"`
	Status     string    `json:"status"`
	Images     []ReviewImage `json:"images,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ReviewImage is a single image attached to a review.
type ReviewImage struct {
	ID          uint64    `json:"id"`
	ReviewID    uint64    `json:"review_id"`
	Filename    string    `json:"filename"`
	StoragePath string    `json:"storage_path"`
	CreatedAt   time.Time `json:"created_at"`
}

// ReviewReport is an abuse report against a review.
type ReviewReport struct {
	ID         uint64    `json:"id"`
	ReviewID   uint64    `json:"review_id"`
	ReporterID uint64    `json:"reporter_id"`
	Reason     string    `json:"reason"`
	Details    string    `json:"details,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// ReviewSummary is aggregated review metrics for an offering.
type ReviewSummary struct {
	TotalReviews  int     `json:"total_reviews"`
	AverageRating float64 `json:"average_rating"`
	PositiveRate  float64 `json:"positive_rate"`
}
