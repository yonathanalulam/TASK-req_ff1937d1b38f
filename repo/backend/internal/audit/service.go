// Package audit writes append-only entries to the audit_logs table.
//
// The service exposes a single Write method called by handlers, services, and
// background jobs. There is intentionally no Update or Delete method: audit
// rows are immutable. A scheduled retention job (Phase 9) prunes rows older
// than 7 years.
package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/eagle-point/service-portal/internal/models"
)

// Service provides audit log writes + reads.
type Service struct {
	db *sql.DB
}

// NewService creates a Service.
func NewService(db *sql.DB) *Service { return &Service{db: db} }

// Entry is the input payload for Write.
// All fields are optional except Action.
type Entry struct {
	UserID     *uint64
	Action     string
	EntityType string
	EntityID   *uint64
	IPAddress  string
	UserAgent  string
	Metadata   map[string]interface{}
}

// Write inserts an entry. Errors are returned but typically swallowed by callers
// because audit failure must not block the underlying business action.
func (s *Service) Write(ctx context.Context, e Entry) error {
	if e.Action == "" {
		return fmt.Errorf("audit: action is required")
	}
	var metaJSON []byte
	if len(e.Metadata) > 0 {
		var err error
		metaJSON, err = json.Marshal(e.Metadata)
		if err != nil {
			return fmt.Errorf("audit: marshal metadata: %w", err)
		}
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit_logs (user_id, action, entity_type, entity_id, ip_address, user_agent, metadata)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		nullableUint64(e.UserID), e.Action, nullableString(e.EntityType),
		nullableUint64(e.EntityID), nullableString(e.IPAddress),
		nullableString(e.UserAgent), nullableBytes(metaJSON),
	)
	if err != nil {
		return fmt.Errorf("audit.Write: %w", err)
	}
	return nil
}

// List returns recent audit entries for a user (or all users if userID == 0).
func (s *Service) List(ctx context.Context, userID uint64, limit int) ([]*models.AuditLog, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := `SELECT id, user_id, action, entity_type, entity_id, ip_address, user_agent, metadata, created_at
	      FROM audit_logs`
	args := []interface{}{}
	if userID > 0 {
		q += ` WHERE user_id = ?`
		args = append(args, userID)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("audit.List: %w", err)
	}
	defer rows.Close()

	var out []*models.AuditLog
	for rows.Next() {
		a := &models.AuditLog{}
		var uid, eid sql.NullInt64
		var entityType, ip, ua sql.NullString
		var meta sql.NullString
		if err := rows.Scan(&a.ID, &uid, &a.Action, &entityType, &eid, &ip, &ua, &meta, &a.CreatedAt); err != nil {
			return nil, err
		}
		if uid.Valid {
			id := uint64(uid.Int64)
			a.UserID = &id
		}
		if eid.Valid {
			id := uint64(eid.Int64)
			a.EntityID = &id
		}
		if entityType.Valid {
			a.EntityType = entityType.String
		}
		if ip.Valid {
			a.IPAddress = ip.String
		}
		if ua.Valid {
			a.UserAgent = ua.String
		}
		if meta.Valid && meta.String != "" {
			_ = json.Unmarshal([]byte(meta.String), &a.Metadata)
		}
		out = append(out, a)
	}
	if out == nil {
		out = []*models.AuditLog{}
	}
	return out, rows.Err()
}

// PurgeBefore deletes audit entries older than `cutoff`. Used by the retention job.
// Returns the number of rows deleted.
func (s *Service) PurgeBefore(ctx context.Context, cutoff string) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM audit_logs WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("audit.PurgeBefore: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func nullableUint64(v *uint64) any {
	if v == nil {
		return nil
	}
	return *v
}
func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func nullableBytes(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}
