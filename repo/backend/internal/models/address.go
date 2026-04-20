package models

import "time"

// Address represents a user's saved address entry.
type Address struct {
	ID           uint64    `json:"id"`
	UserID       uint64    `json:"user_id"`
	Label        string    `json:"label"`
	AddressLine1 string    `json:"address_line1"` // decrypted at read time
	AddressLine2 string    `json:"address_line2"` // decrypted at read time (may be empty)
	City         string    `json:"city"`
	State        string    `json:"state"`
	Zip          string    `json:"zip"`
	IsDefault    bool      `json:"is_default"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
