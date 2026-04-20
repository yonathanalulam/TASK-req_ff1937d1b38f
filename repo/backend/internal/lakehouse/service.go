// Package lakehouse implements Bronze/Silver/Gold layer file management,
// metadata catalog, lineage tracking, lifecycle policies, and legal holds.
//
// Files are written to disk under repo/storage/lakehouse/<layer>/<source>/<date>/.
// All metadata is recorded in the lakehouse_metadata table; lineage links
// between layers live in lakehouse_lineage.
package lakehouse

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/eagle-point/service-portal/internal/models"
)

// Errors.
var (
	ErrNotFound  = errors.New("not found")
	ErrLegalHold = errors.New("operation blocked by legal hold")
)

// Service implements the lakehouse operations.
type Service struct {
	db      *sql.DB
	baseDir string // base path, e.g. "storage/lakehouse"
	backupDir string // archive path, e.g. "storage/backups/lakehouse"
}

// NewService creates a Service.
func NewService(db *sql.DB, baseDir, backupDir string) *Service {
	if baseDir == "" {
		baseDir = "storage/lakehouse"
	}
	if backupDir == "" {
		backupDir = "storage/backups/lakehouse"
	}
	return &Service{db: db, baseDir: baseDir, backupDir: backupDir}
}

// ─── Bronze ──────────────────────────────────────────────────────────────────

// WriteBronze writes raw payload bytes for a source as a Bronze file and
// records the metadata row. Returns the inserted metadata.
func (s *Service) WriteBronze(ctx context.Context, sourceID uint64, payload []byte, rowCount uint64) (*models.LakehouseMetadata, error) {
	return s.write(ctx, sourceID, models.LayerBronze, payload, rowCount, nil)
}

// WriteSilver writes a Silver-layer file linked to the given Bronze inputs.
func (s *Service) WriteSilver(ctx context.Context, sourceID uint64, payload []byte, rowCount uint64, bronzeInputs []uint64) (*models.LakehouseMetadata, error) {
	return s.write(ctx, sourceID, models.LayerSilver, payload, rowCount, bronzeInputs)
}

// WriteGold writes a Gold-layer file linked to Silver inputs.
func (s *Service) WriteGold(ctx context.Context, sourceID uint64, payload []byte, rowCount uint64, silverInputs []uint64) (*models.LakehouseMetadata, error) {
	return s.write(ctx, sourceID, models.LayerGold, payload, rowCount, silverInputs)
}

func (s *Service) write(ctx context.Context, sourceID uint64, layer string, payload []byte, rowCount uint64, inputIDs []uint64) (*models.LakehouseMetadata, error) {
	date := time.Now().UTC().Format("2006-01-02")
	dir := filepath.Join(s.baseDir, layer, strconv.FormatUint(sourceID, 10), date)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("lakehouse.write: mkdir: %w", err)
	}
	fileName := fmt.Sprintf("%d.dat", time.Now().UTC().UnixNano())
	full := filepath.Join(dir, fileName)
	if err := os.WriteFile(full, payload, 0o644); err != nil {
		return nil, fmt.Errorf("lakehouse.write: file: %w", err)
	}

	hash := sha256.Sum256(payload)
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO lakehouse_metadata (source_id, layer, file_path, row_count, schema_hash)
		 VALUES (?, ?, ?, ?, ?)`,
		sourceID, layer, full, rowCount, hex.EncodeToString(hash[:]),
	)
	if err != nil {
		return nil, fmt.Errorf("lakehouse.write: metadata: %w", err)
	}
	id, _ := res.LastInsertId()

	// Insert lineage rows
	for _, in := range inputIDs {
		_, err := s.db.ExecContext(ctx,
			`INSERT IGNORE INTO lakehouse_lineage (output_id, input_id) VALUES (?, ?)`,
			uint64(id), in,
		)
		if err != nil {
			return nil, fmt.Errorf("lakehouse.write: lineage: %w", err)
		}
	}

	return s.GetMetadata(ctx, uint64(id))
}

// ─── Catalog ─────────────────────────────────────────────────────────────────

// GetMetadata fetches a single metadata row.
func (s *Service) GetMetadata(ctx context.Context, id uint64) (*models.LakehouseMetadata, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, source_id, layer, file_path, row_count, schema_hash, ingested_at, archived_at, purged_at
		 FROM lakehouse_metadata WHERE id = ?`, id)
	m, err := scanMetadata(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return m, err
}

// ListCatalog returns metadata rows, optionally filtered by source/layer.
func (s *Service) ListCatalog(ctx context.Context, sourceID uint64, layer string, limit int) ([]*models.LakehouseMetadata, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := `SELECT id, source_id, layer, file_path, row_count, schema_hash, ingested_at, archived_at, purged_at
	      FROM lakehouse_metadata`
	conds := []string{}
	args := []interface{}{}
	if sourceID > 0 {
		conds = append(conds, `source_id = ?`)
		args = append(args, sourceID)
	}
	if layer != "" {
		conds = append(conds, `layer = ?`)
		args = append(args, layer)
	}
	if len(conds) > 0 {
		q += ` WHERE ` + joinAnd(conds)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("lakehouse.ListCatalog: %w", err)
	}
	defer rows.Close()
	var out []*models.LakehouseMetadata
	for rows.Next() {
		m, err := scanMetadata(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if out == nil {
		out = []*models.LakehouseMetadata{}
	}
	return out, rows.Err()
}

// ─── Lineage ─────────────────────────────────────────────────────────────────

// LineageNode is a metadata row with its direct inputs (used for graph rendering).
type LineageNode struct {
	Metadata *models.LakehouseMetadata `json:"metadata"`
	Inputs   []*LineageNode            `json:"inputs,omitempty"`
}

// Lineage returns the full input chain for an output metadata id, recursively.
// Cycles are guarded by a depth cap.
func (s *Service) Lineage(ctx context.Context, outputID uint64) (*LineageNode, error) {
	return s.buildLineage(ctx, outputID, 8)
}

func (s *Service) buildLineage(ctx context.Context, id uint64, depth int) (*LineageNode, error) {
	m, err := s.GetMetadata(ctx, id)
	if err != nil {
		return nil, err
	}
	node := &LineageNode{Metadata: m}
	if depth <= 0 {
		return node, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT input_id FROM lakehouse_lineage WHERE output_id = ?`, id)
	if err != nil {
		return node, nil
	}
	defer rows.Close()
	var inputs []uint64
	for rows.Next() {
		var iid uint64
		_ = rows.Scan(&iid)
		inputs = append(inputs, iid)
	}
	for _, iid := range inputs {
		child, err := s.buildLineage(ctx, iid, depth-1)
		if err == nil {
			node.Inputs = append(node.Inputs, child)
		}
	}
	return node, nil
}

// ─── Lifecycle ───────────────────────────────────────────────────────────────

// LifecycleResult summarises a single sweep.
type LifecycleResult struct {
	Archived int `json:"archived"`
	Purged   int `json:"purged"`
	Held     int `json:"held"`
}

// RunLifecycle executes the daily archive + purge + hold logic.
//
//   - archiveDays: files older than this in Bronze get moved to backupDir.
//   - purgeDays: archived files older than this get deleted from disk.
//   - Files referenced by an active legal hold are skipped (counted in Held).
func (s *Service) RunLifecycle(ctx context.Context, archiveDays, purgeDays int) (*LifecycleResult, error) {
	res := &LifecycleResult{}

	// Active legal holds — collect blocked source/job ids
	holdSources, _, err := s.activeHoldFilter(ctx)
	if err != nil {
		return nil, err
	}

	// Archive: bronze rows older than archiveDays AND not yet archived
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, source_id, file_path FROM lakehouse_metadata
		 WHERE layer='bronze' AND archived_at IS NULL
		   AND ingested_at < DATE_SUB(NOW(), INTERVAL ? DAY)`,
		archiveDays,
	)
	if err != nil {
		return nil, fmt.Errorf("lakehouse.RunLifecycle: archive query: %w", err)
	}
	type metaRow struct {
		id       uint64
		sourceID uint64
		path     string
	}
	var toArchive []metaRow
	for rows.Next() {
		var m metaRow
		if err := rows.Scan(&m.id, &m.sourceID, &m.path); err == nil {
			toArchive = append(toArchive, m)
		}
	}
	rows.Close()

	for _, m := range toArchive {
		if _, blocked := holdSources[m.sourceID]; blocked {
			res.Held++
			continue
		}
		if err := s.archiveFile(ctx, m.id, m.sourceID, m.path); err != nil {
			continue
		}
		res.Archived++
	}

	// Purge: archived rows older than purgeDays
	purgeRows, err := s.db.QueryContext(ctx,
		`SELECT id, source_id, file_path FROM lakehouse_metadata
		 WHERE archived_at IS NOT NULL AND purged_at IS NULL
		   AND archived_at < DATE_SUB(NOW(), INTERVAL ? DAY)`,
		purgeDays,
	)
	if err != nil {
		return res, fmt.Errorf("lakehouse.RunLifecycle: purge query: %w", err)
	}
	var toPurge []metaRow
	for purgeRows.Next() {
		var m metaRow
		if err := purgeRows.Scan(&m.id, &m.sourceID, &m.path); err == nil {
			toPurge = append(toPurge, m)
		}
	}
	purgeRows.Close()

	for _, m := range toPurge {
		if _, blocked := holdSources[m.sourceID]; blocked {
			res.Held++
			continue
		}
		_ = os.Remove(m.path)
		_, _ = s.db.ExecContext(ctx,
			`UPDATE lakehouse_metadata SET purged_at = NOW() WHERE id = ?`, m.id,
		)
		res.Purged++
	}

	return res, nil
}

func (s *Service) archiveFile(ctx context.Context, metaID, sourceID uint64, srcPath string) error {
	rel, err := filepath.Rel(s.baseDir, srcPath)
	if err != nil {
		rel = filepath.Base(srcPath)
	}
	dstPath := filepath.Join(s.backupDir, rel)
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}
	if err := os.Rename(srcPath, dstPath); err != nil {
		// Fallback: copy + delete (in case of cross-device)
		data, rerr := os.ReadFile(srcPath)
		if rerr != nil {
			return rerr
		}
		if werr := os.WriteFile(dstPath, data, 0o644); werr != nil {
			return werr
		}
		_ = os.Remove(srcPath)
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE lakehouse_metadata SET archived_at = NOW(), file_path = ? WHERE id = ?`,
		dstPath, metaID,
	)
	_ = sourceID
	return err
}

// ─── Legal holds ─────────────────────────────────────────────────────────────

// PlaceHold records a new legal hold.
func (s *Service) PlaceHold(ctx context.Context, sourceID, jobID *uint64, reason string, placedBy uint64) (*models.LegalHold, error) {
	if sourceID == nil && jobID == nil {
		return nil, fmt.Errorf("legal hold requires source_id or job_id")
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO legal_holds (source_id, job_id, reason, placed_by) VALUES (?, ?, ?, ?)`,
		nullableUint64(sourceID), nullableUint64(jobID), reason, placedBy,
	)
	if err != nil {
		return nil, fmt.Errorf("lakehouse.PlaceHold: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.GetHold(ctx, uint64(id))
}

// GetHold returns a single legal hold.
func (s *Service) GetHold(ctx context.Context, id uint64) (*models.LegalHold, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, source_id, job_id, reason, placed_by, placed_at, released_at
		 FROM legal_holds WHERE id = ?`, id)
	return scanHold(row)
}

// ListActiveHolds returns currently active (not-yet-released) holds.
func (s *Service) ListActiveHolds(ctx context.Context) ([]*models.LegalHold, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, source_id, job_id, reason, placed_by, placed_at, released_at
		 FROM legal_holds WHERE released_at IS NULL ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("lakehouse.ListActiveHolds: %w", err)
	}
	defer rows.Close()
	var out []*models.LegalHold
	for rows.Next() {
		h, err := scanHold(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	if out == nil {
		out = []*models.LegalHold{}
	}
	return out, rows.Err()
}

// ReleaseHold marks the hold as released.
func (s *Service) ReleaseHold(ctx context.Context, id uint64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE legal_holds SET released_at = NOW() WHERE id = ? AND released_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("lakehouse.ReleaseHold: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// activeHoldFilter returns sets of source_ids and job_ids that are currently
// blocked by an active legal hold.
func (s *Service) activeHoldFilter(ctx context.Context) (map[uint64]struct{}, map[uint64]struct{}, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT source_id, job_id FROM legal_holds WHERE released_at IS NULL`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	sources := map[uint64]struct{}{}
	jobs := map[uint64]struct{}{}
	for rows.Next() {
		var sid, jid sql.NullInt64
		if err := rows.Scan(&sid, &jid); err != nil {
			return nil, nil, err
		}
		if sid.Valid {
			sources[uint64(sid.Int64)] = struct{}{}
		}
		if jid.Valid {
			jobs[uint64(jid.Int64)] = struct{}{}
		}
	}
	return sources, jobs, rows.Err()
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func nullableUint64(v *uint64) any {
	if v == nil {
		return nil
	}
	return *v
}

func joinAnd(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " AND "
		}
		out += p
	}
	return out
}

type scanner interface {
	Scan(dest ...any) error
}

func scanMetadata(rs scanner) (*models.LakehouseMetadata, error) {
	m := &models.LakehouseMetadata{}
	var schemaHash sql.NullString
	var archivedAt, purgedAt sql.NullTime
	if err := rs.Scan(&m.ID, &m.SourceID, &m.Layer, &m.FilePath, &m.RowCount,
		&schemaHash, &m.IngestedAt, &archivedAt, &purgedAt); err != nil {
		return nil, err
	}
	if schemaHash.Valid {
		m.SchemaHash = schemaHash.String
	}
	if archivedAt.Valid {
		t := archivedAt.Time
		m.ArchivedAt = &t
	}
	if purgedAt.Valid {
		t := purgedAt.Time
		m.PurgedAt = &t
	}
	return m, nil
}

func scanHold(rs scanner) (*models.LegalHold, error) {
	h := &models.LegalHold{}
	var sid, jid sql.NullInt64
	var released sql.NullTime
	if err := rs.Scan(&h.ID, &sid, &jid, &h.Reason, &h.PlacedBy, &h.PlacedAt, &released); err != nil {
		return nil, err
	}
	if sid.Valid {
		v := uint64(sid.Int64)
		h.SourceID = &v
	}
	if jid.Valid {
		v := uint64(jid.Int64)
		h.JobID = &v
	}
	if released.Valid {
		t := released.Time
		h.ReleasedAt = &t
	}
	return h, nil
}
