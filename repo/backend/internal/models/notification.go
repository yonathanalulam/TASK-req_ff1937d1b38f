package models

import "time"

// Notification template codes used by the dispatch service.
const (
	NotifTicketStatusChange = "ticket_status_change"
	NotifSLABreach          = "sla_breach"
	NotifAccountLockout     = "account_lockout"
	NotifPostingFreeze      = "posting_freeze"
	NotifApprovalReminder   = "approval_reminder"
	NotifUpcomingStart      = "upcoming_start"
	NotifUpcomingEnd        = "upcoming_end"
)

// NotificationTemplate is an admin-editable title/body pair (Go text/template syntax).
type NotificationTemplate struct {
	ID            uint64    `json:"id"`
	Code          string    `json:"code"`
	TitleTemplate string    `json:"title_template"`
	BodyTemplate  string    `json:"body_template"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Notification is a single in-app message delivered to a user.
type Notification struct {
	ID           uint64    `json:"id"`
	UserID       uint64    `json:"user_id"`
	TemplateCode string    `json:"template_code,omitempty"`
	Title        string    `json:"title"`
	Body         string    `json:"body"`
	IsRead       bool      `json:"is_read"`
	CreatedAt    time.Time `json:"created_at"`
}

// NotificationOutboxEntry is a message routed to the outbox because the
// recipient has disabled in-app notifications.
type NotificationOutboxEntry struct {
	ID             uint64     `json:"id"`
	UserID         uint64     `json:"user_id"`
	NotificationID uint64     `json:"notification_id"`
	Status         string     `json:"status"`
	Attempts       int        `json:"attempts"`
	LastAttemptAt  *time.Time `json:"last_attempt_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	// Optional embedded notification for joined outbox reads
	Notification *Notification `json:"notification,omitempty"`
}
