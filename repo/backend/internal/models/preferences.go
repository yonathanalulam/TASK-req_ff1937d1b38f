package models

import "time"

// Preferences holds per-user notification and content-filtering settings.
type Preferences struct {
	UserID       uint64    `json:"user_id"`
	NotifyInApp  bool      `json:"notify_in_app"`
	MutedTags    []int64   `json:"muted_tags"`
	MutedAuthors []int64   `json:"muted_authors"`
	UpdatedAt    time.Time `json:"updated_at"`
}
