package models

import "time"

// Session represents an authenticated user session stored in the database.
type Session struct {
	ID           string
	UserID       uint64
	CSRFToken    string
	IPAddress    string
	UserAgent    string
	LastActiveAt time.Time
	ExpiresAt    time.Time
	CreatedAt    time.Time
}

// IsExpired returns true if the session has passed its absolute expiry.
func (s *Session) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// IsInactive returns true if the session has been idle longer than the given timeout.
func (s *Session) IsInactive(timeout time.Duration) bool {
	return time.Since(s.LastActiveAt) > timeout
}
