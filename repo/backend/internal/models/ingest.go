package models

import "time"

// Ingest source types.
const (
	SourceDBTable        = "db_table"
	SourceLogFile        = "log_file"
	SourceFilesystemDrop = "filesystem_drop"
)

// Job statuses.
const (
	JobStatusPending   = "pending"
	JobStatusRunning   = "running"
	JobStatusCompleted = "completed"
	JobStatusFailed    = "failed"
	JobStatusPaused    = "paused"
)

// Checkpoint types.
const (
	CheckpointUpdatedAt = "updated_at"
	CheckpointOffset    = "offset"
)

// IngestSource is a registered data source.
type IngestSource struct {
	ID         uint64    `json:"id"`
	Name       string    `json:"name"`
	SourceType string    `json:"source_type"`
	Config     string    `json:"config,omitempty"` // decrypted JSON; empty when listing without secret
	IsActive   bool      `json:"is_active"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// IngestJob is a single execution attempt for a source.
type IngestJob struct {
	ID            uint64     `json:"id"`
	SourceID      uint64     `json:"source_id"`
	Status        string     `json:"status"`
	RowsIngested  uint64     `json:"rows_ingested"`
	RowsExpected  uint64     `json:"rows_expected"`
	SchemaValid   bool       `json:"schema_valid"`
	ErrorMessage  string     `json:"error_message,omitempty"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// IngestCheckpoint records resumable progress for a job.
type IngestCheckpoint struct {
	ID              uint64    `json:"id"`
	SourceID        uint64    `json:"source_id"`
	JobID           uint64    `json:"job_id"`
	CheckpointType  string    `json:"checkpoint_type"`
	CheckpointValue string    `json:"checkpoint_value"`
	UpdatedAt       time.Time `json:"updated_at"`
}
