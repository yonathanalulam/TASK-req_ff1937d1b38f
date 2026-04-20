package models

import "time"

// Q&A post status constants.
const (
	QAStatusPublished         = "published"
	QAStatusPendingModeration = "pending_moderation"
	QAStatusClosed            = "closed"
	QAPostStatusRemoved       = "removed"
)

// QAThread is a top-level question under an offering.
type QAThread struct {
	ID         uint64    `json:"id"`
	OfferingID uint64    `json:"offering_id"`
	AuthorID   uint64    `json:"author_id"`
	Question   string    `json:"question"`
	Status     string    `json:"status"`
	Replies    []QAPost  `json:"replies,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// QAPost is a reply under a Q&A thread.
type QAPost struct {
	ID        uint64    `json:"id"`
	ThreadID  uint64    `json:"thread_id"`
	AuthorID  uint64    `json:"author_id"`
	Content   string    `json:"content"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
