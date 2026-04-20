package models

import "time"

// Address represents a user's saved address entry.
type Address struct {
	ID           uint64
	UserID       uint64
	Label        string
	AddressLine1 string // decrypted at read time
	AddressLine2 string // decrypted at read time (may be empty)
	City         string
	State        string
	Zip          string
	IsDefault    bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
