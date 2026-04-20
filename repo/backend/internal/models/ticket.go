package models

import "time"

// Ticket status constants.
const (
	TicketStatusAccepted   = "Accepted"
	TicketStatusDispatched = "Dispatched"
	TicketStatusInService  = "In Service"
	TicketStatusCompleted  = "Completed"
	TicketStatusClosed     = "Closed"
	TicketStatusCancelled  = "Cancelled"
)

// Delivery method constants.
const (
	DeliveryPickup  = "pickup"
	DeliveryCourier = "courier"
)

// Ticket represents a service request.
type Ticket struct {
	ID              uint64     `json:"id"`
	UserID          uint64     `json:"user_id"`
	AssignedAgentID *uint64    `json:"assigned_agent_id,omitempty"`
	OfferingID      uint64     `json:"offering_id"`
	CategoryID      uint64     `json:"category_id"`
	AddressID       uint64     `json:"address_id"`
	PreferredStart  time.Time  `json:"preferred_start"`
	PreferredEnd    time.Time  `json:"preferred_end"`
	DeliveryMethod  string     `json:"delivery_method"`
	ShippingFee     float64    `json:"shipping_fee"`
	Status          string     `json:"status"`
	SLADeadline     *time.Time `json:"sla_deadline,omitempty"`
	SLABreached     bool       `json:"sla_breached"`
	CancelReason    string     `json:"cancel_reason,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// TicketNote is a single note attached to a ticket.
type TicketNote struct {
	ID        uint64    `json:"id"`
	TicketID  uint64    `json:"ticket_id"`
	AuthorID  uint64    `json:"author_id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// TicketAttachment is a file linked to a ticket.
type TicketAttachment struct {
	ID           uint64    `json:"id"`
	TicketID     uint64    `json:"ticket_id"`
	Filename     string    `json:"filename"`
	OriginalName string    `json:"original_name"`
	MimeType     string    `json:"mime_type"`
	SizeBytes    uint64    `json:"size_bytes"`
	StoragePath  string    `json:"storage_path"`
	CreatedAt    time.Time `json:"created_at"`
}
