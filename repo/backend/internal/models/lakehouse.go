package models

import "time"

// Lakehouse layers.
const (
	LayerBronze = "bronze"
	LayerSilver = "silver"
	LayerGold   = "gold"
)

// LakehouseMetadata represents a single ingested file at a layer.
type LakehouseMetadata struct {
	ID         uint64     `json:"id"`
	SourceID   uint64     `json:"source_id"`
	Layer      string     `json:"layer"`
	FilePath   string     `json:"file_path"`
	RowCount   uint64     `json:"row_count"`
	SchemaHash string     `json:"schema_hash,omitempty"`
	IngestedAt time.Time  `json:"ingested_at"`
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
	PurgedAt   *time.Time `json:"purged_at,omitempty"`
}

// LakehouseLineage links output → input metadata rows.
type LakehouseLineage struct {
	ID        uint64    `json:"id"`
	OutputID  uint64    `json:"output_id"`
	InputID   uint64    `json:"input_id"`
	CreatedAt time.Time `json:"created_at"`
}

// LakehouseSchemaVersion records a snapshot of a source's schema.
type LakehouseSchemaVersion struct {
	ID         uint64    `json:"id"`
	SourceID   uint64    `json:"source_id"`
	Version    int       `json:"version"`
	SchemaJSON string    `json:"schema_json"`
	IsBreaking bool      `json:"is_breaking"`
	CreatedAt  time.Time `json:"created_at"`
}

// LakehouseLifecyclePolicy is a per-source / per-layer retention rule.
type LakehouseLifecyclePolicy struct {
	ID              uint64    `json:"id"`
	SourceID        *uint64   `json:"source_id,omitempty"`
	Layer           string    `json:"layer"`
	ArchiveAfterDays int      `json:"archive_after_days"`
	PurgeAfterDays  int       `json:"purge_after_days"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// LegalHold prevents purge of files associated with a source or job.
type LegalHold struct {
	ID         uint64     `json:"id"`
	SourceID   *uint64    `json:"source_id,omitempty"`
	JobID      *uint64    `json:"job_id,omitempty"`
	Reason     string     `json:"reason"`
	PlacedBy   uint64     `json:"placed_by"`
	PlacedAt   time.Time  `json:"placed_at"`
	ReleasedAt *time.Time `json:"released_at,omitempty"`
}
