package models

import "time"

// ServiceCategory represents a top-level service grouping.
type ServiceCategory struct {
	ID                    uint64    `json:"id"`
	Name                  string    `json:"name"`
	Slug                  string    `json:"slug"`
	Description           string    `json:"description"`
	ResponseTimeMinutes   int       `json:"response_time_minutes"`
	CompletionTimeMinutes int       `json:"completion_time_minutes"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// ServiceOffering represents a single purchasable service.
type ServiceOffering struct {
	ID              uint64    `json:"id"`
	AgentID         uint64    `json:"agent_id"`
	CategoryID      uint64    `json:"category_id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	BasePrice       float64   `json:"base_price"`
	DurationMinutes int       `json:"duration_minutes"`
	ActiveStatus    bool      `json:"active_status"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
