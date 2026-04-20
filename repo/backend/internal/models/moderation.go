package models

import "time"

// Sensitive term classes.
const (
	TermClassProhibited = "prohibited"
	TermClassBorderline = "borderline"
)

// Moderation queue statuses.
const (
	ModStatusPending  = "pending"
	ModStatusApproved = "approved"
	ModStatusRejected = "rejected"
)

// Moderation content types.
const (
	ModContentReview     = "review"
	ModContentQAThread   = "qa_thread"
	ModContentQAPost     = "qa_post"
	ModContentTicketNote = "ticket_note"
)

// SensitiveTerm is a single dictionary entry.
type SensitiveTerm struct {
	ID        uint64    `json:"id"`
	Term      string    `json:"term"`
	Class     string    `json:"class"` // "prohibited" | "borderline"
	CreatedAt time.Time `json:"created_at"`
}

// ModerationQueueItem is a flagged piece of content awaiting review.
type ModerationQueueItem struct {
	ID           uint64     `json:"id"`
	ContentType  string     `json:"content_type"`
	ContentID    uint64     `json:"content_id"`
	ContentText  string     `json:"content_text"`
	FlaggedTerms []string   `json:"flagged_terms,omitempty"`
	Status       string     `json:"status"`
	ModeratorID  *uint64    `json:"moderator_id,omitempty"`
	ReviewedAt   *time.Time `json:"reviewed_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// ModerationAction records an approval/rejection decision.
type ModerationAction struct {
	ID          uint64    `json:"id"`
	ModeratorID uint64    `json:"moderator_id"`
	ActionType  string    `json:"action_type"`
	ContentType string    `json:"content_type"`
	ContentID   uint64    `json:"content_id"`
	Reason      string    `json:"reason,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// ViolationRecord tracks a single rejected content violation for a user.
type ViolationRecord struct {
	ID                  uint64    `json:"id"`
	UserID              uint64    `json:"user_id"`
	ContentType         string    `json:"content_type"`
	ContentID           uint64    `json:"content_id"`
	ViolationAt         time.Time `json:"violation_at"`
	FreezeApplied       bool      `json:"freeze_applied"`
	FreezeDurationHours int       `json:"freeze_duration_hours"`
	CreatedAt           time.Time `json:"created_at"`
}
