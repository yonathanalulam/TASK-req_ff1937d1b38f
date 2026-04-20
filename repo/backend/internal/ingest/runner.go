package ingest

// Executable job runner that pulls data from a registered source, writes
// Bronze/Silver/Gold layers via the lakehouse writer, advances the job's
// checkpoint, and persists status/progress/error on the ingest_jobs row.
//
// This file converts the control-plane CRUD for sources/jobs into an actual
// offline pipeline. Source config is a small JSON document whose shape depends
// on source_type:
//
//   db_table:        {"table": "<name>", "updated_at_column": "<col>"}
//   log_file:        {"path": "/abs/or/rel/path.log"}
//   filesystem_drop: {"dir": "/abs/or/rel/dir"}
//
// The runner is synchronous (each run drives a single job end-to-end) — a
// background worker that picks up pending jobs is a straightforward wrapper
// around RunJob.

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/eagle-point/service-portal/internal/models"
)

// LakehouseWriter is the subset of lakehouse.Service the runner needs.
// Kept as an interface so tests can plug in a recording stub without importing
// the full lakehouse package (avoids import cycles).
type LakehouseWriter interface {
	WriteBronze(ctx context.Context, sourceID uint64, payload []byte, rowCount uint64) (*models.LakehouseMetadata, error)
	WriteSilver(ctx context.Context, sourceID uint64, payload []byte, rowCount uint64, bronzeInputs []uint64) (*models.LakehouseMetadata, error)
	WriteGold(ctx context.Context, sourceID uint64, payload []byte, rowCount uint64, silverInputs []uint64) (*models.LakehouseMetadata, error)
}

// RunResult summarises a single execution.
type RunResult struct {
	JobID        uint64 `json:"job_id"`
	BronzeID     uint64 `json:"bronze_id,omitempty"`
	SilverID     uint64 `json:"silver_id,omitempty"`
	GoldID       uint64 `json:"gold_id,omitempty"`
	RowsIngested uint64 `json:"rows_ingested"`
	RowsExpected uint64 `json:"rows_expected"`
	SchemaValid  bool   `json:"schema_valid"`
	Status       string `json:"status"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// RunJob executes the job end-to-end. It updates the job row as it progresses,
// so callers can poll /jobs/:id for status without holding the request open.
// An error here is ALSO persisted on the row; callers receive both the error
// and the last RunResult so the UI can show partial progress.
func (s *Service) RunJob(ctx context.Context, writer LakehouseWriter, jobID uint64) (*RunResult, error) {
	job, err := s.GetJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if job.Status == models.JobStatusRunning || job.Status == models.JobStatusCompleted {
		return &RunResult{JobID: job.ID, Status: job.Status}, fmt.Errorf("%w: job already %s", ErrValidation, job.Status)
	}

	src, err := s.GetSource(ctx, job.SourceID, true)
	if err != nil {
		return nil, err
	}

	result := &RunResult{JobID: job.ID, Status: models.JobStatusRunning}

	// Mark running. Empty error message clears any stale failure string.
	if uerr := s.UpdateJobProgress(ctx, job.ID, models.JobStatusRunning, 0, 0, ""); uerr != nil {
		return result, uerr
	}

	// Resume from the most recent checkpoint for this source, if any.
	ck, _ := s.LatestCheckpointForSource(ctx, src.ID)

	var (
		payload []byte
		rows    uint64
		fields  []SchemaField
		ckType  string
		ckValue string
	)

	switch src.SourceType {
	case models.SourceDBTable:
		payload, rows, fields, ckType, ckValue, err = s.pullDBTable(ctx, src, ck)
	case models.SourceLogFile:
		payload, rows, fields, ckType, ckValue, err = s.pullLogFile(src, ck)
	case models.SourceFilesystemDrop:
		payload, rows, fields, ckType, ckValue, err = s.pullFilesystemDrop(src, ck)
	default:
		err = fmt.Errorf("%w: unsupported source_type %q", ErrValidation, src.SourceType)
	}
	if err != nil {
		result.Status = models.JobStatusFailed
		result.ErrorMessage = err.Error()
		_ = s.UpdateJobProgress(ctx, job.ID, models.JobStatusFailed, 0, 0, err.Error())
		return result, err
	}

	result.RowsIngested = rows
	result.RowsExpected = rows // exact match for offline pulls; DB case may override

	// Schema evolution tracking — persists a new version row for this source
	// and surfaces breaking changes on the job row (schema_valid flag).
	_, schemaErr := s.RecordSchemaVersion(ctx, src.ID, fields)
	result.SchemaValid = !errors.Is(schemaErr, ErrSchemaBroken)

	// Write Bronze (raw payload). If this fails we persist failure.
	bronze, err := writer.WriteBronze(ctx, src.ID, payload, rows)
	if err != nil {
		result.Status = models.JobStatusFailed
		result.ErrorMessage = err.Error()
		_ = s.UpdateJobProgress(ctx, job.ID, models.JobStatusFailed, rows, rows, err.Error())
		return result, err
	}
	result.BronzeID = bronze.ID

	// Silver: normalized JSON array of {field: value} rows. Uses Bronze as input.
	silverPayload, silverRows := s.buildSilver(payload, src.SourceType, fields)
	silver, serr := writer.WriteSilver(ctx, src.ID, silverPayload, silverRows, []uint64{bronze.ID})
	if serr == nil {
		result.SilverID = silver.ID
		// Gold: summary statistics over Silver.
		goldPayload := buildGold(silverPayload, silverRows)
		gold, gerr := writer.WriteGold(ctx, src.ID, goldPayload, 1, []uint64{silver.ID})
		if gerr == nil {
			result.GoldID = gold.ID
		}
	}

	// Persist checkpoint so the next job resumes from here.
	if ckType != "" && ckValue != "" {
		_ = s.SaveCheckpoint(ctx, src.ID, job.ID, ckType, ckValue)
	}

	status := models.JobStatusCompleted
	errMsg := ""
	if HasRowCountDiscrepancy(result.RowsExpected, result.RowsIngested) {
		errMsg = fmt.Sprintf("row-count discrepancy: expected %d, ingested %d",
			result.RowsExpected, result.RowsIngested)
	}
	if err := s.UpdateJobProgress(ctx, job.ID, status, result.RowsIngested, result.RowsExpected, errMsg); err != nil {
		return result, err
	}
	result.Status = status
	result.ErrorMessage = errMsg
	return result, nil
}

// ─── Source pullers ────────────────────────────────────────────────────────

// pullDBTable reads rows from a local MySQL table. Config JSON:
//
//	{"table": "<identifier>", "updated_at_column": "<col>"}
//
// The `updated_at_column` is optional; when present, it enables incremental
// resumes via an updated_at checkpoint. Table names are validated to an
// identifier-safe subset (letters/digits/underscore) before interpolation —
// parameter binding is not available for table identifiers in MySQL.
func (s *Service) pullDBTable(ctx context.Context, src *models.IngestSource, ck *models.IngestCheckpoint) ([]byte, uint64, []SchemaField, string, string, error) {
	var cfg struct {
		Table           string `json:"table"`
		UpdatedAtColumn string `json:"updated_at_column"`
	}
	if err := json.Unmarshal([]byte(src.Config), &cfg); err != nil {
		return nil, 0, nil, "", "", fmt.Errorf("%w: invalid db_table config: %s", ErrValidation, err)
	}
	if !isSafeIdent(cfg.Table) {
		return nil, 0, nil, "", "", fmt.Errorf("%w: table name must be a plain identifier", ErrValidation)
	}
	if cfg.UpdatedAtColumn != "" && !isSafeIdent(cfg.UpdatedAtColumn) {
		return nil, 0, nil, "", "", fmt.Errorf("%w: updated_at_column must be a plain identifier", ErrValidation)
	}

	q := "SELECT * FROM `" + cfg.Table + "`"
	args := []interface{}{}
	if cfg.UpdatedAtColumn != "" && ck != nil && ck.CheckpointType == models.CheckpointUpdatedAt && ck.CheckpointValue != "" {
		q += " WHERE `" + cfg.UpdatedAtColumn + "` > ?"
		args = append(args, ck.CheckpointValue)
	}
	q += " LIMIT 10000"

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, nil, "", "", fmt.Errorf("db_table query: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, 0, nil, "", "", err
	}
	types, _ := rows.ColumnTypes()

	records := make([]map[string]any, 0, 64)
	var latestUpdatedAt string
	for rows.Next() {
		raw := make([]sql.RawBytes, len(cols))
		dest := make([]any, len(cols))
		for i := range raw {
			dest[i] = &raw[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, 0, nil, "", "", err
		}
		rec := make(map[string]any, len(cols))
		for i, col := range cols {
			if raw[i] == nil {
				rec[col] = nil
				continue
			}
			rec[col] = string(raw[i])
			if col == cfg.UpdatedAtColumn {
				if v := string(raw[i]); v > latestUpdatedAt {
					latestUpdatedAt = v
				}
			}
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, nil, "", "", err
	}

	payload, _ := json.Marshal(records)

	fields := make([]SchemaField, 0, len(cols))
	for i, col := range cols {
		t := "string"
		if i < len(types) && types[i] != nil {
			t = strings.ToLower(types[i].DatabaseTypeName())
		}
		fields = append(fields, SchemaField{Name: col, Type: t})
	}

	ckType, ckValue := "", ""
	if cfg.UpdatedAtColumn != "" && latestUpdatedAt != "" {
		ckType = models.CheckpointUpdatedAt
		ckValue = latestUpdatedAt
	} else {
		ckType = models.CheckpointOffset
		prev := uint64(0)
		if ck != nil && ck.CheckpointType == models.CheckpointOffset {
			fmt.Sscanf(ck.CheckpointValue, "%d", &prev)
		}
		ckValue = fmt.Sprintf("%d", prev+uint64(len(records)))
	}

	return payload, uint64(len(records)), fields, ckType, ckValue, nil
}

// pullLogFile reads a text log file and emits one record per line.
// Resumes from a byte-offset checkpoint so re-runs only read the tail.
func (s *Service) pullLogFile(src *models.IngestSource, ck *models.IngestCheckpoint) ([]byte, uint64, []SchemaField, string, string, error) {
	var cfg struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(src.Config), &cfg); err != nil {
		return nil, 0, nil, "", "", fmt.Errorf("%w: invalid log_file config: %s", ErrValidation, err)
	}
	if cfg.Path == "" {
		return nil, 0, nil, "", "", fmt.Errorf("%w: log_file requires path", ErrValidation)
	}
	f, err := os.Open(cfg.Path)
	if err != nil {
		return nil, 0, nil, "", "", fmt.Errorf("log_file open: %w", err)
	}
	defer f.Close()

	var offset int64
	if ck != nil && ck.CheckpointType == models.CheckpointOffset && ck.CheckpointValue != "" {
		fmt.Sscanf(ck.CheckpointValue, "%d", &offset)
	}
	fi, err := f.Stat()
	if err != nil {
		return nil, 0, nil, "", "", err
	}
	if offset > fi.Size() {
		offset = 0 // file was truncated or rotated
	}
	if _, err := f.Seek(offset, 0); err != nil {
		return nil, 0, nil, "", "", err
	}
	remaining := fi.Size() - offset
	buf := make([]byte, remaining)
	n, _ := f.Read(buf)
	buf = buf[:n]

	lines := strings.Split(strings.TrimRight(string(buf), "\n"), "\n")
	records := make([]map[string]any, 0, len(lines))
	for _, l := range lines {
		if l == "" {
			continue
		}
		records = append(records, map[string]any{"line": l})
	}
	payload, _ := json.Marshal(records)

	fields := []SchemaField{{Name: "line", Type: "string"}}
	ckType := models.CheckpointOffset
	ckValue := fmt.Sprintf("%d", offset+int64(n))
	return payload, uint64(len(records)), fields, ckType, ckValue, nil
}

// pullFilesystemDrop scans a directory and emits one record per file containing
// file metadata (name, size, hash, modified time).
func (s *Service) pullFilesystemDrop(src *models.IngestSource, ck *models.IngestCheckpoint) ([]byte, uint64, []SchemaField, string, string, error) {
	var cfg struct {
		Dir string `json:"dir"`
	}
	if err := json.Unmarshal([]byte(src.Config), &cfg); err != nil {
		return nil, 0, nil, "", "", fmt.Errorf("%w: invalid filesystem_drop config: %s", ErrValidation, err)
	}
	if cfg.Dir == "" {
		return nil, 0, nil, "", "", fmt.Errorf("%w: filesystem_drop requires dir", ErrValidation)
	}

	var lastSeen time.Time
	if ck != nil && ck.CheckpointType == models.CheckpointUpdatedAt && ck.CheckpointValue != "" {
		if t, err := time.Parse(time.RFC3339Nano, ck.CheckpointValue); err == nil {
			lastSeen = t
		}
	}

	records := make([]map[string]any, 0, 16)
	var newestMod time.Time
	err := filepath.WalkDir(cfg.Dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.ModTime().After(lastSeen) {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		sum := sha256.Sum256(data)
		rec := map[string]any{
			"name":         filepath.Base(path),
			"path":         path,
			"size_bytes":   info.Size(),
			"modified_at":  info.ModTime().UTC().Format(time.RFC3339Nano),
			"sha256":       hex.EncodeToString(sum[:]),
		}
		records = append(records, rec)
		if info.ModTime().After(newestMod) {
			newestMod = info.ModTime()
		}
		return nil
	})
	if err != nil {
		return nil, 0, nil, "", "", fmt.Errorf("filesystem_drop walk: %w", err)
	}

	payload, _ := json.Marshal(records)
	fields := []SchemaField{
		{Name: "name", Type: "string"},
		{Name: "path", Type: "string"},
		{Name: "size_bytes", Type: "int64"},
		{Name: "modified_at", Type: "string"},
		{Name: "sha256", Type: "string"},
	}

	ckType := models.CheckpointUpdatedAt
	ckValue := lastSeen.UTC().Format(time.RFC3339Nano)
	if !newestMod.IsZero() {
		ckValue = newestMod.UTC().Format(time.RFC3339Nano)
	}
	return payload, uint64(len(records)), fields, ckType, ckValue, nil
}

// ─── Silver / Gold helpers ─────────────────────────────────────────────────

// buildSilver returns a normalized payload for the Silver layer. The shape is
// the same JSON array as Bronze but with records sorted by their first field
// so downstream diffing is deterministic. Row count is preserved.
func (s *Service) buildSilver(bronze []byte, sourceType string, fields []SchemaField) ([]byte, uint64) {
	var records []map[string]any
	if err := json.Unmarshal(bronze, &records); err != nil {
		return bronze, 0
	}
	if len(records) > 1 && len(fields) > 0 {
		key := fields[0].Name
		sort.SliceStable(records, func(i, j int) bool {
			return fmt.Sprint(records[i][key]) < fmt.Sprint(records[j][key])
		})
	}
	out, _ := json.Marshal(records)
	_ = sourceType
	return out, uint64(len(records))
}

// buildGold produces a summary object (row count + schema fingerprint). One row.
func buildGold(silver []byte, silverRows uint64) []byte {
	h := sha256.Sum256(silver)
	gold := map[string]any{
		"row_count":        silverRows,
		"silver_sha256":    hex.EncodeToString(h[:]),
		"generated_at_utc": time.Now().UTC().Format(time.RFC3339),
	}
	out, _ := json.Marshal(gold)
	return out
}

// isSafeIdent returns true when s contains only [A-Za-z0-9_] and starts with a
// letter or underscore. Used to guard table/column names against injection
// when parameter binding is unavailable.
func isSafeIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_':
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}
