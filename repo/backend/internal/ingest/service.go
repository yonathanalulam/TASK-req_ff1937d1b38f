// Package ingest implements the data ingestion source registry, job engine,
// resumable checkpoints, and schema-evolution detection described in Phase 10.
//
// The implementation is offline-only: jobs read from local DB tables, log
// files, or filesystem drops, and write Bronze layer files to the local disk.
// All routes are HMAC-protected (wired in the router).
package ingest

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/eagle-point/service-portal/internal/crypto"
	"github.com/eagle-point/service-portal/internal/models"
)

// Errors.
var (
	ErrNotFound      = errors.New("not found")
	ErrValidation    = errors.New("validation error")
	ErrSchemaBroken  = errors.New("breaking schema change detected")
)

// Service provides ingestion source + job + checkpoint logic.
type Service struct {
	db     *sql.DB
	encKey string // AES-256 key for encrypting source connection_config
}

// NewService creates a Service.
func NewService(db *sql.DB, encKey string) *Service { return &Service{db: db, encKey: encKey} }

// ─── Sources ─────────────────────────────────────────────────────────────────

// CreateSourceInput carries fields for a new source.
type CreateSourceInput struct {
	Name       string
	SourceType string
	Config     string // JSON-encoded connection details (will be encrypted at rest)
}

// CreateSource registers a new source. Config is AES-encrypted before storage.
func (s *Service) CreateSource(ctx context.Context, in CreateSourceInput) (*models.IngestSource, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrValidation)
	}
	if in.SourceType != models.SourceDBTable && in.SourceType != models.SourceLogFile && in.SourceType != models.SourceFilesystemDrop {
		return nil, fmt.Errorf("%w: source_type must be db_table, log_file, or filesystem_drop", ErrValidation)
	}

	enc, err := s.encryptConfig(in.Config)
	if err != nil {
		return nil, err
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO ingest_sources (name, source_type, config_encrypted) VALUES (?, ?, ?)`,
		in.Name, in.SourceType, enc,
	)
	if err != nil {
		return nil, fmt.Errorf("ingest.CreateSource: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.GetSource(ctx, uint64(id), false)
}

// UpdateSource modifies fields and re-encrypts config.
func (s *Service) UpdateSource(ctx context.Context, id uint64, in CreateSourceInput, isActive bool) (*models.IngestSource, error) {
	enc, err := s.encryptConfig(in.Config)
	if err != nil {
		return nil, err
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE ingest_sources
		 SET name=?, source_type=?, config_encrypted=?, is_active=?
		 WHERE id=?`,
		strings.TrimSpace(in.Name), in.SourceType, enc, isActive, id,
	)
	if err != nil {
		return nil, fmt.Errorf("ingest.UpdateSource: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetSource(ctx, id, false)
}

// GetSource fetches a source. If includeConfig=true, the config is decrypted
// and returned; otherwise it is omitted.
func (s *Service) GetSource(ctx context.Context, id uint64, includeConfig bool) (*models.IngestSource, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, source_type, config_encrypted, is_active, created_at, updated_at
		 FROM ingest_sources WHERE id = ?`, id)
	src, err := s.scanSource(row, includeConfig)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return src, err
}

// ListSources returns all registered sources (no decrypted config).
func (s *Service) ListSources(ctx context.Context) ([]*models.IngestSource, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, source_type, config_encrypted, is_active, created_at, updated_at
		 FROM ingest_sources ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("ingest.ListSources: %w", err)
	}
	defer rows.Close()

	var out []*models.IngestSource
	for rows.Next() {
		src, err := s.scanSource(rows, false)
		if err != nil {
			return nil, err
		}
		out = append(out, src)
	}
	if out == nil {
		out = []*models.IngestSource{}
	}
	return out, rows.Err()
}

// ─── Jobs ────────────────────────────────────────────────────────────────────

// CreateJob inserts a new pending job.
func (s *Service) CreateJob(ctx context.Context, sourceID uint64) (*models.IngestJob, error) {
	if _, err := s.GetSource(ctx, sourceID, false); err != nil {
		return nil, err
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO ingest_jobs (source_id, status) VALUES (?, 'pending')`, sourceID,
	)
	if err != nil {
		return nil, fmt.Errorf("ingest.CreateJob: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.GetJob(ctx, uint64(id))
}

// GetJob returns a job by id.
func (s *Service) GetJob(ctx context.Context, id uint64) (*models.IngestJob, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, source_id, status, rows_ingested, rows_expected, schema_valid,
		        error_message, started_at, completed_at, created_at
		 FROM ingest_jobs WHERE id = ?`, id)
	return scanJob(row)
}

// ListJobs returns recent jobs (newest first), optionally filtered by source.
func (s *Service) ListJobs(ctx context.Context, sourceID uint64, limit int) ([]*models.IngestJob, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `SELECT id, source_id, status, rows_ingested, rows_expected, schema_valid,
	             error_message, started_at, completed_at, created_at
	      FROM ingest_jobs`
	args := []interface{}{}
	if sourceID > 0 {
		q += ` WHERE source_id = ?`
		args = append(args, sourceID)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("ingest.ListJobs: %w", err)
	}
	defer rows.Close()

	var out []*models.IngestJob
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	if out == nil {
		out = []*models.IngestJob{}
	}
	return out, rows.Err()
}

// UpdateJobProgress advances a job's status, row counts, error message.
func (s *Service) UpdateJobProgress(ctx context.Context, id uint64, status string, rowsIngested, rowsExpected uint64, errMsg string) error {
	var startedSet string
	if status == models.JobStatusRunning {
		startedSet = `, started_at = COALESCE(started_at, NOW())`
	}
	if status == models.JobStatusCompleted || status == models.JobStatusFailed {
		startedSet = `, completed_at = NOW()`
	}
	q := fmt.Sprintf(
		`UPDATE ingest_jobs
		 SET status=?, rows_ingested=?, rows_expected=?, error_message=?%s
		 WHERE id=?`, startedSet)
	_, err := s.db.ExecContext(ctx, q, status, rowsIngested, rowsExpected, nullIfEmpty(errMsg), id)
	return err
}

// ─── Checkpoints ─────────────────────────────────────────────────────────────

// SaveCheckpoint upserts the (source_id, job_id) checkpoint row.
func (s *Service) SaveCheckpoint(ctx context.Context, sourceID, jobID uint64, ckType, value string) error {
	if ckType != models.CheckpointUpdatedAt && ckType != models.CheckpointOffset {
		return fmt.Errorf("%w: invalid checkpoint type", ErrValidation)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO ingest_checkpoints (source_id, job_id, checkpoint_type, checkpoint_value)
		 VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE checkpoint_type=VALUES(checkpoint_type),
		                         checkpoint_value=VALUES(checkpoint_value)`,
		sourceID, jobID, ckType, value,
	)
	return err
}

// LoadCheckpoint returns the most recent checkpoint for a (source, job).
func (s *Service) LoadCheckpoint(ctx context.Context, sourceID, jobID uint64) (*models.IngestCheckpoint, error) {
	c := &models.IngestCheckpoint{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, source_id, job_id, checkpoint_type, checkpoint_value, updated_at
		 FROM ingest_checkpoints WHERE source_id=? AND job_id=?`,
		sourceID, jobID,
	).Scan(&c.ID, &c.SourceID, &c.JobID, &c.CheckpointType, &c.CheckpointValue, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return c, err
}

// LatestCheckpointForSource returns the most recently updated checkpoint across
// all jobs for a source. Used to resume incremental ingestion.
func (s *Service) LatestCheckpointForSource(ctx context.Context, sourceID uint64) (*models.IngestCheckpoint, error) {
	c := &models.IngestCheckpoint{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, source_id, job_id, checkpoint_type, checkpoint_value, updated_at
		 FROM ingest_checkpoints WHERE source_id=?
		 ORDER BY updated_at DESC LIMIT 1`,
		sourceID,
	).Scan(&c.ID, &c.SourceID, &c.JobID, &c.CheckpointType, &c.CheckpointValue, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return c, err
}

// ─── Row-count integrity ─────────────────────────────────────────────────────

// RowCountDiscrepancyTolerance is the fractional discrepancy that triggers a warning.
// 0.001 = 0.1%, matching the spec.
const RowCountDiscrepancyTolerance = 0.001

// HasRowCountDiscrepancy returns true when |expected - ingested| / expected
// exceeds RowCountDiscrepancyTolerance.
func HasRowCountDiscrepancy(expected, ingested uint64) bool {
	if expected == 0 {
		return ingested != 0
	}
	diff := int64(expected) - int64(ingested)
	if diff < 0 {
		diff = -diff
	}
	return float64(diff)/float64(expected) > RowCountDiscrepancyTolerance
}

// ─── Schema evolution ────────────────────────────────────────────────────────

// SchemaField describes a single column in a source schema.
type SchemaField struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// IsBreakingSchemaChange returns true when going from `old` to `new` would
// remove a column or narrow an existing column's type. Adding new columns is
// always considered backward-compatible.
//
// Type narrowing rules:
//   - "int64" → "int32" is breaking
//   - "string" → "int" is breaking
//   - "*" → same type is fine
func IsBreakingSchemaChange(oldFields, newFields []SchemaField) bool {
	newByName := map[string]string{}
	for _, f := range newFields {
		newByName[f.Name] = f.Type
	}
	for _, of := range oldFields {
		nt, ok := newByName[of.Name]
		if !ok {
			return true // column removed
		}
		if isTypeNarrowing(of.Type, nt) {
			return true
		}
	}
	return false
}

func isTypeNarrowing(oldType, newType string) bool {
	if oldType == newType {
		return false
	}
	// Any string → numeric (or vice versa) is breaking.
	stringy := func(t string) bool {
		return t == "string" || t == "text" || t == "varchar"
	}
	numeric := func(t string) bool {
		return t == "int" || t == "int32" || t == "int64" ||
			t == "bigint" || t == "float" || t == "double" || t == "decimal"
	}
	if stringy(oldType) != stringy(newType) {
		return true
	}
	if numeric(oldType) && numeric(newType) {
		// Treat any change between distinct numeric types as narrowing — the
		// caller (a real schema evolution policy) can refine this later.
		return true
	}
	return true
}

// SchemaHash returns a deterministic hash of the schema for change detection.
func SchemaHash(fields []SchemaField) string {
	// Stable order
	cp := append([]SchemaField(nil), fields...)
	sort.Slice(cp, func(i, j int) bool { return cp[i].Name < cp[j].Name })
	b, _ := json.Marshal(cp)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// RecordSchemaVersion stores a new schema snapshot for a source. If the schema
// is identical to the current latest version, nothing is inserted and the
// existing row is returned. If breaking, returns ErrSchemaBroken AFTER inserting
// the row (with is_breaking=1) so the caller can see what tripped.
func (s *Service) RecordSchemaVersion(ctx context.Context, sourceID uint64, fields []SchemaField) (*models.LakehouseSchemaVersion, error) {
	// Find latest version
	var latestVer int
	var latestSchema sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT version, schema_json FROM lakehouse_schema_versions
		 WHERE source_id=? ORDER BY version DESC LIMIT 1`, sourceID,
	).Scan(&latestVer, &latestSchema)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("ingest.RecordSchemaVersion: %w", err)
	}

	newJSON, _ := json.Marshal(fields)
	if latestSchema.Valid && string(newJSON) == latestSchema.String {
		// No change — return current
		row := s.db.QueryRowContext(ctx,
			`SELECT id, source_id, version, schema_json, is_breaking, created_at
			 FROM lakehouse_schema_versions WHERE source_id=? AND version=?`,
			sourceID, latestVer)
		return scanSchemaVersion(row)
	}

	isBreaking := false
	if latestSchema.Valid {
		var oldFields []SchemaField
		_ = json.Unmarshal([]byte(latestSchema.String), &oldFields)
		isBreaking = IsBreakingSchemaChange(oldFields, fields)
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO lakehouse_schema_versions (source_id, version, schema_json, is_breaking)
		 VALUES (?, ?, ?, ?)`,
		sourceID, latestVer+1, newJSON, isBreaking,
	)
	if err != nil {
		return nil, fmt.Errorf("ingest.RecordSchemaVersion: insert: %w", err)
	}
	id, _ := res.LastInsertId()
	row := s.db.QueryRowContext(ctx,
		`SELECT id, source_id, version, schema_json, is_breaking, created_at
		 FROM lakehouse_schema_versions WHERE id=?`, id)
	v, err := scanSchemaVersion(row)
	if err != nil {
		return nil, err
	}
	if isBreaking {
		return v, ErrSchemaBroken
	}
	return v, nil
}

// ListSchemaVersions returns all version rows for a source (oldest first).
func (s *Service) ListSchemaVersions(ctx context.Context, sourceID uint64) ([]*models.LakehouseSchemaVersion, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, source_id, version, schema_json, is_breaking, created_at
		 FROM lakehouse_schema_versions WHERE source_id=? ORDER BY version ASC`, sourceID)
	if err != nil {
		return nil, fmt.Errorf("ingest.ListSchemaVersions: %w", err)
	}
	defer rows.Close()
	var out []*models.LakehouseSchemaVersion
	for rows.Next() {
		v, err := scanSchemaVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if out == nil {
		out = []*models.LakehouseSchemaVersion{}
	}
	return out, rows.Err()
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func (s *Service) encryptConfig(plain string) ([]byte, error) {
	if s.encKey == "" {
		// Test mode — store raw bytes
		return []byte(plain), nil
	}
	if plain == "" {
		return []byte{}, nil
	}
	return crypto.EncryptString(plain, s.encKey)
}

func (s *Service) decryptConfig(data []byte) (string, error) {
	if len(data) == 0 {
		return "", nil
	}
	if s.encKey == "" {
		return string(data), nil
	}
	return crypto.DecryptString(data, s.encKey)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func (s *Service) scanSource(rs rowScanner, includeConfig bool) (*models.IngestSource, error) {
	src := &models.IngestSource{}
	var encConfig []byte
	if err := rs.Scan(&src.ID, &src.Name, &src.SourceType, &encConfig,
		&src.IsActive, &src.CreatedAt, &src.UpdatedAt); err != nil {
		return nil, err
	}
	if includeConfig {
		conf, err := s.decryptConfig(encConfig)
		if err != nil {
			return nil, err
		}
		src.Config = conf
	}
	return src, nil
}

func scanJob(rs rowScanner) (*models.IngestJob, error) {
	j := &models.IngestJob{}
	var errMsg sql.NullString
	var startedAt, completedAt sql.NullTime
	if err := rs.Scan(&j.ID, &j.SourceID, &j.Status, &j.RowsIngested, &j.RowsExpected,
		&j.SchemaValid, &errMsg, &startedAt, &completedAt, &j.CreatedAt); err != nil {
		return nil, err
	}
	if errMsg.Valid {
		j.ErrorMessage = errMsg.String
	}
	if startedAt.Valid {
		t := startedAt.Time
		j.StartedAt = &t
	}
	if completedAt.Valid {
		t := completedAt.Time
		j.CompletedAt = &t
	}
	return j, nil
}

func scanSchemaVersion(rs rowScanner) (*models.LakehouseSchemaVersion, error) {
	v := &models.LakehouseSchemaVersion{}
	if err := rs.Scan(&v.ID, &v.SourceID, &v.Version, &v.SchemaJSON, &v.IsBreaking, &v.CreatedAt); err != nil {
		return nil, err
	}
	return v, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
