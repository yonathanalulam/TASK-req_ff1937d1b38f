package models

import "time"

// Audit log action constants (free-form strings; enumerated here for grep-ability).
const (
	AuditActionLogin             = "login"
	AuditActionLogout            = "logout"
	AuditActionLockout           = "account_lockout"
	AuditActionRegister          = "register"
	AuditActionProfileUpdate     = "profile_update"
	AuditActionAddressCreate     = "address_create"
	AuditActionAddressUpdate     = "address_update"
	AuditActionAddressDelete     = "address_delete"
	AuditActionTicketCreate      = "ticket_create"
	AuditActionTicketTransition  = "ticket_transition"
	AuditActionModerationDecide  = "moderation_decide"
	AuditActionAdminOp           = "admin_operation"
	AuditActionExportRequested   = "data_export_requested"
	AuditActionExportDownloaded  = "data_export_downloaded"
	AuditActionDeletionRequested = "data_deletion_requested"
	AuditActionDeletionApplied   = "data_deletion_applied"
)

// Data export request statuses.
const (
	ExportStatusPending    = "pending"
	ExportStatusProcessing = "processing"
	ExportStatusReady      = "ready"
	ExportStatusDownloaded = "downloaded"
	ExportStatusExpired    = "expired"
)

// Data deletion request statuses.
const (
	DeletionStatusPending    = "pending"
	DeletionStatusAnonymized = "anonymized"
	DeletionStatusCancelled  = "cancelled"
)

// AuditLog is a single append-only entry.
type AuditLog struct {
	ID         uint64                 `json:"id"`
	UserID     *uint64                `json:"user_id,omitempty"`
	Action     string                 `json:"action"`
	EntityType string                 `json:"entity_type,omitempty"`
	EntityID   *uint64                `json:"entity_id,omitempty"`
	IPAddress  string                 `json:"ip_address,omitempty"`
	UserAgent  string                 `json:"user_agent,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
}

// DataExportRequest tracks a pending or completed data export.
type DataExportRequest struct {
	ID           uint64     `json:"id"`
	UserID       uint64     `json:"user_id"`
	Status       string     `json:"status"`
	FilePath     string     `json:"file_path,omitempty"`
	RequestedAt  time.Time  `json:"requested_at"`
	ReadyAt      *time.Time `json:"ready_at,omitempty"`
	DownloadedAt *time.Time `json:"downloaded_at,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
}

// DataDeletionRequest tracks a pending or applied account deletion.
type DataDeletionRequest struct {
	ID           uint64     `json:"id"`
	UserID       uint64     `json:"user_id"`
	Status       string     `json:"status"`
	RequestedAt  time.Time  `json:"requested_at"`
	ScheduledFor time.Time  `json:"scheduled_for"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}
