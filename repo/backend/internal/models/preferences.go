package models

import "time"

// Preferences holds per-user notification and content-filtering settings.
type Preferences struct {
	UserID       uint64
	NotifyInApp  bool
	MutedTags    []int64
	MutedAuthors []int64
	UpdatedAt    time.Time
}
