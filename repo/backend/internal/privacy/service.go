// Package privacy implements user-initiated data export and deletion flows.
//
// Export pipeline:
//   1. POST /users/me/export-request inserts a row in 'pending' status.
//   2. A background worker (started by the router) picks up pending rows,
//      writes a ZIP under storage/exports/:user_id/, and flips status to 'ready'.
//   3. GET /users/me/export-request/status returns current status.
//   4. GET /users/me/export-request/download streams the ZIP and flips to 'downloaded'.
//   5. After 48 hours, a cleanup pass purges the file and marks 'expired'.
//
// Deletion pipeline:
//   1. POST /users/me/deletion-request immediately deactivates the user
//      (is_active=0) and schedules anonymization 30 days out.
//   2. A daily worker selects rows past their scheduled_for and applies
//      Anonymize: phone NULL, address lines NULL, display_name "Deleted User",
//      email replaced with a hash. Audit log entries keep the user_id.
package privacy

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/eagle-point/service-portal/internal/audit"
	"github.com/eagle-point/service-portal/internal/bgjob"
	"github.com/eagle-point/service-portal/internal/models"
)

// Errors.
var (
	ErrNotFound       = errors.New("not found")
	ErrAlreadyPending = errors.New("an active request already exists")
	ErrNotReady       = errors.New("export not ready yet")
)

// Tunables (overridable in tests).
var (
	DeletionGracePeriod = 30 * 24 * time.Hour
	ExportRetention     = 48 * time.Hour
	WorkerInterval      = 30 * time.Second // how often the export worker polls
	DeletionInterval    = 1 * time.Hour    // how often the deletion worker polls
)

// Service orchestrates export + deletion flows.
type Service struct {
	db        *sql.DB
	auditSvc  *audit.Service
	exportDir string // base directory: e.g., "storage/exports"
}

// NewService creates a Service.
func NewService(db *sql.DB, auditSvc *audit.Service, exportDir string) *Service {
	if exportDir == "" {
		exportDir = "storage/exports"
	}
	return &Service{db: db, auditSvc: auditSvc, exportDir: exportDir}
}

// ─── Export ──────────────────────────────────────────────────────────────────

// RequestExport creates a pending export request. Only one active request
// (pending/processing/ready) per user is allowed at a time.
func (s *Service) RequestExport(ctx context.Context, userID uint64) (*models.DataExportRequest, error) {
	// Check for existing active request
	var existingID uint64
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM data_export_requests
		 WHERE user_id = ? AND status IN ('pending','processing','ready')
		 LIMIT 1`, userID,
	).Scan(&existingID)
	if err == nil {
		return nil, ErrAlreadyPending
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("privacy.RequestExport: %w", err)
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO data_export_requests (user_id, status) VALUES (?, 'pending')`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("privacy.RequestExport: %w", err)
	}
	id, _ := res.LastInsertId()

	if s.auditSvc != nil {
		_ = s.auditSvc.Write(ctx, audit.Entry{
			UserID: ptrUint64(userID), Action: models.AuditActionExportRequested,
			EntityType: "export_request", EntityID: ptrUint64(uint64(id)),
		})
	}
	return s.GetActiveExport(ctx, userID)
}

// GetActiveExport returns the user's active export, or ErrNotFound.
func (s *Service) GetActiveExport(ctx context.Context, userID uint64) (*models.DataExportRequest, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, status, file_path, requested_at, ready_at, downloaded_at, expires_at
		 FROM data_export_requests
		 WHERE user_id = ?
		 ORDER BY id DESC LIMIT 1`, userID,
	)
	return scanExport(row)
}

// MarkDownloaded flips the request to 'downloaded' once the ZIP is streamed.
func (s *Service) MarkDownloaded(ctx context.Context, requestID uint64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE data_export_requests SET status='downloaded', downloaded_at=NOW() WHERE id=?`,
		requestID,
	)
	return err
}

// GenerateExport builds the ZIP file for a single export request and updates the
// row to 'ready'. Exposed so tests can drive the pipeline synchronously.
func (s *Service) GenerateExport(ctx context.Context, requestID uint64) error {
	// Move to processing
	_, err := s.db.ExecContext(ctx,
		`UPDATE data_export_requests SET status='processing' WHERE id = ? AND status='pending'`,
		requestID,
	)
	if err != nil {
		return fmt.Errorf("privacy.GenerateExport: claim: %w", err)
	}

	req, err := s.getExportByID(ctx, requestID)
	if err != nil {
		return err
	}

	dir := filepath.Join(s.exportDir, strconv.FormatUint(req.UserID, 10))
	// 0o750: owner rwx, group rx, other nothing. Exports contain PII; even
	// on a single-tenant box we don't want world-readable directories.
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("privacy.GenerateExport: mkdir: %w", err)
	}
	zipPath := filepath.Join(dir, fmt.Sprintf("export-%d.zip", req.ID))

	if err := s.writeUserZip(ctx, req.UserID, zipPath); err != nil {
		_, _ = s.db.ExecContext(ctx,
			`UPDATE data_export_requests SET status='pending' WHERE id=?`, requestID)
		return err
	}

	expires := time.Now().Add(ExportRetention)
	_, err = s.db.ExecContext(ctx,
		`UPDATE data_export_requests SET status='ready', file_path=?, ready_at=NOW(), expires_at=? WHERE id=?`,
		zipPath, expires, requestID,
	)
	return err
}

// writeUserZip aggregates all user-owned data into a single ZIP file.
// Each entity type lands in its own JSON file inside the archive.
func (s *Service) writeUserZip(ctx context.Context, userID uint64, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("privacy.writeUserZip: create: %w", err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	// Profile
	profile, _ := s.collectProfile(ctx, userID)
	if err := writeJSON(zw, "profile.json", profile); err != nil {
		return err
	}
	// Addresses
	addrs, _ := s.collectAddresses(ctx, userID)
	if err := writeJSON(zw, "addresses.json", addrs); err != nil {
		return err
	}
	// Tickets + notes
	tickets, _ := s.collectTickets(ctx, userID)
	if err := writeJSON(zw, "tickets.json", tickets); err != nil {
		return err
	}
	// Reviews
	reviews, _ := s.collectReviews(ctx, userID)
	if err := writeJSON(zw, "reviews.json", reviews); err != nil {
		return err
	}
	// Q&A
	qa, _ := s.collectQA(ctx, userID)
	if err := writeJSON(zw, "qa.json", qa); err != nil {
		return err
	}
	// Notifications
	notifs, _ := s.collectNotifications(ctx, userID)
	if err := writeJSON(zw, "notifications.json", notifs); err != nil {
		return err
	}
	// Audit logs explicitly excluded per spec
	return nil
}

// CollectExportPayload returns the in-memory map written into the ZIP. Exposed
// so tests can verify contents without unzipping.
func (s *Service) CollectExportPayload(ctx context.Context, userID uint64) map[string]any {
	profile, _ := s.collectProfile(ctx, userID)
	addrs, _ := s.collectAddresses(ctx, userID)
	tickets, _ := s.collectTickets(ctx, userID)
	reviews, _ := s.collectReviews(ctx, userID)
	qa, _ := s.collectQA(ctx, userID)
	notifs, _ := s.collectNotifications(ctx, userID)
	return map[string]any{
		"profile":       profile,
		"addresses":     addrs,
		"tickets":       tickets,
		"reviews":       reviews,
		"qa":            qa,
		"notifications": notifs,
	}
}

func (s *Service) collectProfile(ctx context.Context, userID uint64) (map[string]any, error) {
	out := map[string]any{}
	var username, email, displayName string
	var bio, avatar sql.NullString
	if err := s.db.QueryRowContext(ctx,
		`SELECT username, email, display_name, bio, avatar_url FROM users WHERE id=?`, userID,
	).Scan(&username, &email, &displayName, &bio, &avatar); err != nil {
		return out, err
	}
	out["username"] = username
	out["email"] = email
	out["display_name"] = displayName
	if bio.Valid {
		out["bio"] = bio.String
	}
	if avatar.Valid {
		out["avatar_url"] = avatar.String
	}
	return out, nil
}

func (s *Service) collectAddresses(ctx context.Context, userID uint64) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, label, city, state, zip, is_default, created_at
		 FROM addresses WHERE user_id=? ORDER BY id ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		m := map[string]any{}
		var id uint64
		var label, city, state, zip string
		var isDefault bool
		var createdAt time.Time
		if err := rows.Scan(&id, &label, &city, &state, &zip, &isDefault, &createdAt); err != nil {
			return nil, err
		}
		m["id"] = id
		m["label"] = label
		m["city"] = city
		m["state"] = state
		m["zip"] = zip
		m["is_default"] = isDefault
		m["created_at"] = createdAt
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Service) collectTickets(ctx context.Context, userID uint64) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, offering_id, status, preferred_start, preferred_end, created_at
		 FROM tickets WHERE user_id=? ORDER BY id ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, offID uint64
		var status string
		var ps, pe, ca time.Time
		if err := rows.Scan(&id, &offID, &status, &ps, &pe, &ca); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"id":              id,
			"offering_id":     offID,
			"status":          status,
			"preferred_start": ps,
			"preferred_end":   pe,
			"created_at":      ca,
		})
	}
	return out, rows.Err()
}

func (s *Service) collectReviews(ctx context.Context, userID uint64) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, offering_id, rating, text, created_at FROM reviews WHERE user_id=?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, offID uint64
		var rating int
		var text sql.NullString
		var ca time.Time
		if err := rows.Scan(&id, &offID, &rating, &text, &ca); err != nil {
			return nil, err
		}
		row := map[string]any{
			"id":          id,
			"offering_id": offID,
			"rating":      rating,
			"created_at":  ca,
		}
		if text.Valid {
			row["text"] = text.String
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Service) collectQA(ctx context.Context, userID uint64) (map[string]any, error) {
	out := map[string]any{"threads": []map[string]any{}, "posts": []map[string]any{}}

	thRows, err := s.db.QueryContext(ctx,
		`SELECT id, offering_id, question, created_at FROM qa_threads WHERE author_id=?`, userID)
	if err == nil {
		defer thRows.Close()
		threads := []map[string]any{}
		for thRows.Next() {
			var id, offID uint64
			var q string
			var ca time.Time
			if err := thRows.Scan(&id, &offID, &q, &ca); err == nil {
				threads = append(threads, map[string]any{
					"id": id, "offering_id": offID, "question": q, "created_at": ca,
				})
			}
		}
		out["threads"] = threads
	}

	pRows, err := s.db.QueryContext(ctx,
		`SELECT id, thread_id, content, created_at FROM qa_posts WHERE author_id=?`, userID)
	if err == nil {
		defer pRows.Close()
		posts := []map[string]any{}
		for pRows.Next() {
			var id, tid uint64
			var content string
			var ca time.Time
			if err := pRows.Scan(&id, &tid, &content, &ca); err == nil {
				posts = append(posts, map[string]any{
					"id": id, "thread_id": tid, "content": content, "created_at": ca,
				})
			}
		}
		out["posts"] = posts
	}
	return out, nil
}

func (s *Service) collectNotifications(ctx context.Context, userID uint64) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, body, is_read, created_at FROM notifications WHERE user_id=?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id uint64
		var title, body string
		var read bool
		var ca time.Time
		if err := rows.Scan(&id, &title, &body, &read, &ca); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"id": id, "title": title, "body": body, "is_read": read, "created_at": ca,
		})
	}
	return out, rows.Err()
}

// CleanupExpiredExports purges ZIP files past their expires_at and marks rows
// as 'expired'. Returns count of rows updated.
func (s *Service) CleanupExpiredExports(ctx context.Context) (int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, file_path FROM data_export_requests
		 WHERE status='ready' AND expires_at IS NOT NULL AND expires_at < NOW()`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type expired struct {
		id   uint64
		path string
	}
	var batch []expired
	for rows.Next() {
		var e expired
		var path sql.NullString
		if err := rows.Scan(&e.id, &path); err != nil {
			continue
		}
		if path.Valid {
			e.path = path.String
		}
		batch = append(batch, e)
	}
	rows.Close()

	for _, e := range batch {
		// Defense in depth: file_path is always server-generated under
		// exportDir today, but a future bug or migration could land an
		// arbitrary path here. Refuse to os.Remove anything outside the
		// export root so cleanup cannot be weaponised into arbitrary file
		// deletion.
		if e.path != "" {
			if s.isInsideExportDir(e.path) {
				_ = os.Remove(e.path)
			} else {
				log.Printf("privacy.CleanupExpiredExports: refusing to delete %q — outside export dir %q",
					e.path, s.exportDir)
			}
		}
		_, _ = s.db.ExecContext(ctx,
			`UPDATE data_export_requests SET status='expired', file_path=NULL WHERE id=?`,
			e.id,
		)
	}
	return int64(len(batch)), nil
}

// isInsideExportDir reports whether path is a descendant of s.exportDir
// after resolving both to absolute forms. Symlink traversal is not probed
// — the worker runs under a trusted service account and the export dir is
// created by the worker itself; attackers cannot plant symlinks inside it.
// The goal is simply to block path strings that escape the dir via ".." or
// absolute references.
func (s *Service) isInsideExportDir(path string) bool {
	baseAbs, err := filepath.Abs(s.exportDir)
	if err != nil {
		return false
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	// Append separator so /storage/exports does not match /storage/exports-evil.
	sep := string(filepath.Separator)
	return pathAbs == baseAbs || strings.HasPrefix(pathAbs, baseAbs+sep)
}

// ─── Deletion ────────────────────────────────────────────────────────────────

// RequestDeletion immediately deactivates the user and schedules anonymization.
func (s *Service) RequestDeletion(ctx context.Context, userID uint64) (*models.DataDeletionRequest, error) {
	// Ensure no active deletion pending
	var pendingID uint64
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM data_deletion_requests WHERE user_id=? AND status='pending' LIMIT 1`, userID,
	).Scan(&pendingID)
	if err == nil {
		return nil, ErrAlreadyPending
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("privacy.RequestDeletion: %w", err)
	}

	scheduled := time.Now().Add(DeletionGracePeriod)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO data_deletion_requests (user_id, status, scheduled_for) VALUES (?, 'pending', ?)`,
		userID, scheduled,
	)
	if err != nil {
		return nil, fmt.Errorf("privacy.RequestDeletion: %w", err)
	}
	id, _ := res.LastInsertId()

	// Deactivate immediately
	if _, err := tx.ExecContext(ctx, `UPDATE users SET is_active=0 WHERE id=?`, userID); err != nil {
		return nil, fmt.Errorf("privacy.RequestDeletion: deactivate: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	if s.auditSvc != nil {
		_ = s.auditSvc.Write(ctx, audit.Entry{
			UserID: ptrUint64(userID), Action: models.AuditActionDeletionRequested,
			EntityType: "deletion_request", EntityID: ptrUint64(uint64(id)),
		})
	}
	return s.GetActiveDeletion(ctx, userID)
}

// GetActiveDeletion returns the user's most recent deletion request, or ErrNotFound.
func (s *Service) GetActiveDeletion(ctx context.Context, userID uint64) (*models.DataDeletionRequest, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, status, requested_at, scheduled_for, completed_at
		 FROM data_deletion_requests WHERE user_id=?
		 ORDER BY id DESC LIMIT 1`, userID)
	r := &models.DataDeletionRequest{}
	var completed sql.NullTime
	err := row.Scan(&r.ID, &r.UserID, &r.Status, &r.RequestedAt, &r.ScheduledFor, &completed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if completed.Valid {
		t := completed.Time
		r.CompletedAt = &t
	}
	return r, nil
}

// AnonymizeUser is exported so the daily worker AND tests can drive it.
// It blanks personal fields, replaces email with a hash, and marks the user
// as is_deleted=1. Audit logs and content retention are NOT touched (per spec).
func (s *Service) AnonymizeUser(ctx context.Context, userID uint64) error {
	hashedEmail := anonymizedEmail(userID)
	now := time.Now().UTC()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`UPDATE users
		 SET email=?, display_name='Deleted User', phone_encrypted=NULL,
		     bio=NULL, avatar_url=NULL,
		     is_active=0, is_deleted=1, anonymized_at=?
		 WHERE id=?`,
		hashedEmail, now, userID,
	); err != nil {
		return fmt.Errorf("privacy.AnonymizeUser: user: %w", err)
	}

	// Null encrypted address lines
	if _, err := tx.ExecContext(ctx,
		`UPDATE addresses
		 SET address_line1_encrypted=NULL, address_line2_encrypted=NULL
		 WHERE user_id=?`, userID,
	); err != nil {
		// Some installations may have NOT NULL on address_line1_encrypted;
		// fall back to deleting the row.
		if _, derr := tx.ExecContext(ctx, `DELETE FROM addresses WHERE user_id=?`, userID); derr != nil {
			return fmt.Errorf("privacy.AnonymizeUser: addresses: %w", err)
		}
	}

	// Mark the most recent deletion request as anonymized
	if _, err := tx.ExecContext(ctx,
		`UPDATE data_deletion_requests
		 SET status='anonymized', completed_at=?
		 WHERE user_id=? AND status='pending'`,
		now, userID,
	); err != nil {
		return fmt.Errorf("privacy.AnonymizeUser: ddr: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	if s.auditSvc != nil {
		_ = s.auditSvc.Write(ctx, audit.Entry{
			UserID: ptrUint64(userID), Action: models.AuditActionDeletionApplied,
			EntityType: "user", EntityID: ptrUint64(userID),
		})
	}
	return nil
}

// ProcessDueDeletions runs anonymization for every pending deletion past its
// scheduled_for. Returns count processed.
func (s *Service) ProcessDueDeletions(ctx context.Context) (int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT user_id FROM data_deletion_requests
		 WHERE status='pending' AND scheduled_for <= NOW()`)
	if err != nil {
		return 0, err
	}
	var ids []uint64
	for rows.Next() {
		var id uint64
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()

	count := 0
	for _, uid := range ids {
		if err := s.AnonymizeUser(ctx, uid); err != nil {
			log.Printf("privacy: anonymize user %d failed: %v", uid, err)
			continue
		}
		count++
	}
	return count, nil
}

// AdminHardDelete is the immediate admin-driven path. It runs anonymization
// regardless of grace period.
func (s *Service) AdminHardDelete(ctx context.Context, userID uint64, adminID uint64) error {
	if err := s.AnonymizeUser(ctx, userID); err != nil {
		return err
	}
	if s.auditSvc != nil {
		_ = s.auditSvc.Write(ctx, audit.Entry{
			UserID: ptrUint64(adminID), Action: models.AuditActionAdminOp,
			EntityType: "user", EntityID: ptrUint64(userID),
			Metadata: map[string]interface{}{"op": "hard_delete"},
		})
	}
	return nil
}

// ─── Background workers ─────────────────────────────────────────────────────

// StartExportWorker polls for pending export requests and generates ZIPs.
// Also performs cleanup of expired files in the same loop.
//
// Each tick is wrapped in bgjob.Safe so a panic inside any single export
// (malformed DB row, zip writer error, disk full) does not kill the
// goroutine. Losing this goroutine would mean pending export requests pile
// up forever with no user-visible error signal.
func StartExportWorker(ctx context.Context, svc *Service) {
	ticker := time.NewTicker(WorkerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			bgjob.Safe("export-worker-tick", func() {
				processPendingExports(ctx, svc)
				_, _ = svc.CleanupExpiredExports(ctx)
			})
		}
	}
}

func processPendingExports(ctx context.Context, svc *Service) {
	rows, err := svc.db.QueryContext(ctx,
		`SELECT id FROM data_export_requests WHERE status='pending' ORDER BY id ASC LIMIT 10`)
	if err != nil {
		log.Printf("privacy: export poll failed: %v", err)
		return
	}
	var ids []uint64
	for rows.Next() {
		var id uint64
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if err := svc.GenerateExport(ctx, id); err != nil {
			log.Printf("privacy: generate export %d failed: %v", id, err)
		}
	}
}

// StartDeletionWorker periodically processes due deletion requests. Same
// panic-barrier reasoning as StartExportWorker — a nil row or a broken
// Anonymize path must not silently stop the loop.
func StartDeletionWorker(ctx context.Context, svc *Service) {
	ticker := time.NewTicker(DeletionInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			bgjob.Safe("deletion-worker-tick", func() {
				if _, err := svc.ProcessDueDeletions(ctx); err != nil {
					log.Printf("privacy: deletion worker failed: %v", err)
				}
			})
		}
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func writeJSON(zw *zip.Writer, name string, payload any) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

// anonymizedEmail returns a deterministic placeholder so the unique constraint
// on users.email is preserved without leaking the original address.
func anonymizedEmail(userID uint64) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("anonymized-%d", userID)))
	return fmt.Sprintf("anon-%s@deleted.local", hex.EncodeToString(h[:8]))
}

func ptrUint64(v uint64) *uint64 { return &v }

func (s *Service) getExportByID(ctx context.Context, id uint64) (*models.DataExportRequest, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, status, file_path, requested_at, ready_at, downloaded_at, expires_at
		 FROM data_export_requests WHERE id=?`, id)
	return scanExport(row)
}

func scanExport(row interface{ Scan(...any) error }) (*models.DataExportRequest, error) {
	r := &models.DataExportRequest{}
	var filePath sql.NullString
	var readyAt, downloadedAt, expiresAt sql.NullTime
	err := row.Scan(&r.ID, &r.UserID, &r.Status, &filePath, &r.RequestedAt, &readyAt, &downloadedAt, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if filePath.Valid {
		r.FilePath = filePath.String
	}
	if readyAt.Valid {
		t := readyAt.Time
		r.ReadyAt = &t
	}
	if downloadedAt.Valid {
		t := downloadedAt.Time
		r.DownloadedAt = &t
	}
	if expiresAt.Valid {
		t := expiresAt.Time
		r.ExpiresAt = &t
	}
	return r, nil
}
