package privacy_test

import (
	"archive/zip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eagle-point/service-portal/internal/audit"
	"github.com/eagle-point/service-portal/internal/models"
	"github.com/eagle-point/service-portal/internal/privacy"
	"github.com/eagle-point/service-portal/internal/testutil"
)

func setup(t *testing.T) string {
	t.Helper()
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"audit_logs",
		"data_export_requests", "data_deletion_requests",
		"notification_outbox", "notifications", "notification_templates",
		"violation_records", "moderation_actions", "moderation_queue", "sensitive_terms",
		"qa_posts", "qa_threads",
		"review_reports", "review_images", "reviews",
		"ticket_attachments", "ticket_notes", "tickets",
		"service_offerings", "service_categories",
		"addresses", "login_attempts", "sessions", "user_preferences", "user_roles", "users",
	)
	dir, err := os.MkdirTemp("", "privacy-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestRequestExport_PendingThenReady(t *testing.T) {
	db := testutil.DBOrSkip(t)
	exportDir := setup(t)

	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"expuser", "exp@t.l", "$2a$04$p", "Exp")
	var userID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='expuser'`).Scan(&userID)

	svc := privacy.NewService(db, audit.NewService(db), exportDir)

	req, err := svc.RequestExport(context.Background(), userID)
	require.NoError(t, err)
	assert.Equal(t, models.ExportStatusPending, req.Status)

	// Drive worker synchronously
	require.NoError(t, svc.GenerateExport(context.Background(), req.ID))

	updated, err := svc.GetActiveExport(context.Background(), userID)
	require.NoError(t, err)
	assert.Equal(t, models.ExportStatusReady, updated.Status)
	assert.NotEmpty(t, updated.FilePath)

	// Verify ZIP contains the expected files
	r, err := zip.OpenReader(updated.FilePath)
	require.NoError(t, err)
	defer r.Close()

	names := make(map[string]bool)
	for _, f := range r.File {
		names[f.Name] = true
	}
	for _, want := range []string{"profile.json", "addresses.json", "tickets.json",
		"reviews.json", "qa.json", "notifications.json"} {
		assert.True(t, names[want], "ZIP should contain %s", want)
	}
	assert.False(t, names["audit_logs.json"], "ZIP must NOT contain audit logs")
}

func TestRequestExport_DuplicatePendingRejected(t *testing.T) {
	db := testutil.DBOrSkip(t)
	dir := setup(t)
	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"dupexp", "de@t.l", "$2a$04$p", "DE")
	var userID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='dupexp'`).Scan(&userID)

	svc := privacy.NewService(db, nil, dir)
	_, err := svc.RequestExport(context.Background(), userID)
	require.NoError(t, err)
	_, err = svc.RequestExport(context.Background(), userID)
	assert.ErrorIs(t, err, privacy.ErrAlreadyPending)
}

func TestExport_PayloadShape(t *testing.T) {
	db := testutil.DBOrSkip(t)
	dir := setup(t)
	db.Exec(`INSERT INTO users (username, email, password_hash, display_name, bio) VALUES (?,?,?,?,?)`,
		"shapeu", "sh@t.l", "$2a$04$p", "Shape", "hello bio")
	var userID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='shapeu'`).Scan(&userID)

	svc := privacy.NewService(db, nil, dir)
	payload := svc.CollectExportPayload(context.Background(), userID)
	profile, ok := payload["profile"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "shapeu", profile["username"])
	assert.Equal(t, "hello bio", profile["bio"])

	// Marshal to ensure JSON-serializable
	_, err := json.Marshal(payload)
	require.NoError(t, err)
}

func TestRequestDeletion_DeactivatesUserImmediately(t *testing.T) {
	db := testutil.DBOrSkip(t)
	setup(t)
	db.Exec(`INSERT INTO users (username, email, password_hash, display_name, is_active) VALUES (?,?,?,?,1)`,
		"deluser", "del@t.l", "$2a$04$p", "Del")
	var userID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='deluser'`).Scan(&userID)

	svc := privacy.NewService(db, nil, "")
	dr, err := svc.RequestDeletion(context.Background(), userID)
	require.NoError(t, err)
	assert.Equal(t, models.DeletionStatusPending, dr.Status)
	// Scheduled ~30 days out
	assert.True(t, dr.ScheduledFor.After(time.Now().Add(29*24*time.Hour)))

	var active bool
	db.QueryRow(`SELECT is_active FROM users WHERE id=?`, userID).Scan(&active)
	assert.False(t, active, "user should be deactivated immediately on deletion request")
}

func TestAnonymizeUser_BlanksPersonalFields(t *testing.T) {
	db := testutil.DBOrSkip(t)
	setup(t)
	db.Exec(`INSERT INTO users (username, email, password_hash, display_name, bio) VALUES (?,?,?,?,?)`,
		"anonu", "anon@t.l", "$2a$04$p", "Original Name", "private bio")
	var userID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='anonu'`).Scan(&userID)

	svc := privacy.NewService(db, nil, "")
	require.NoError(t, svc.AnonymizeUser(context.Background(), userID))

	var displayName, email string
	var bio []byte
	var isDeleted bool
	db.QueryRow(`SELECT display_name, email, bio, is_deleted FROM users WHERE id=?`, userID).
		Scan(&displayName, &email, &bio, &isDeleted)

	assert.Equal(t, "Deleted User", displayName)
	assert.NotEqual(t, "anon@t.l", email, "email should be replaced with hash placeholder")
	assert.Contains(t, email, "@deleted.local")
	assert.True(t, isDeleted)
}

func TestProcessDueDeletions_RunsScheduledDeletions(t *testing.T) {
	db := testutil.DBOrSkip(t)
	setup(t)
	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"sched", "sc@t.l", "$2a$04$p", "Sc")
	var userID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='sched'`).Scan(&userID)

	// Insert a pending deletion already past its scheduled_for
	db.Exec(`INSERT INTO data_deletion_requests (user_id, status, scheduled_for)
		VALUES (?, 'pending', DATE_SUB(NOW(), INTERVAL 1 HOUR))`, userID)

	svc := privacy.NewService(db, nil, "")
	n, err := svc.ProcessDueDeletions(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	var status string
	db.QueryRow(`SELECT status FROM data_deletion_requests WHERE user_id=?`, userID).Scan(&status)
	assert.Equal(t, "anonymized", status)

	var displayName string
	db.QueryRow(`SELECT display_name FROM users WHERE id=?`, userID).Scan(&displayName)
	assert.Equal(t, "Deleted User", displayName)
}

func TestCleanupExpiredExports(t *testing.T) {
	db := testutil.DBOrSkip(t)
	dir := setup(t)
	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"clu", "cl@t.l", "$2a$04$p", "CL")
	var userID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='clu'`).Scan(&userID)

	// Create a fake ready+expired request with a real file on disk
	tmpFile := filepath.Join(dir, "old.zip")
	require.NoError(t, os.WriteFile(tmpFile, []byte("dummy"), 0o644))
	db.Exec(
		`INSERT INTO data_export_requests (user_id, status, file_path, ready_at, expires_at)
		 VALUES (?, 'ready', ?, NOW(), DATE_SUB(NOW(), INTERVAL 1 HOUR))`,
		userID, tmpFile,
	)

	svc := privacy.NewService(db, nil, dir)
	n, err := svc.CleanupExpiredExports(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
	_, statErr := os.Stat(tmpFile)
	assert.Error(t, statErr, "expired ZIP should be removed")
}

func TestCleanupExpiredExports_RefusesPathsOutsideExportDir(t *testing.T) {
	// Defence in depth: even if a buggy code path or migration writes an
	// arbitrary file_path into data_export_requests, the cleanup worker
	// must not delete it.
	db := testutil.DBOrSkip(t)
	exportDir := setup(t)

	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"traverse", "tr@t.l", "$2a$04$p", "TR")
	var userID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='traverse'`).Scan(&userID)

	// Stage a victim file in a SIBLING temp dir, outside exportDir.
	victimDir, err := os.MkdirTemp("", "victim-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(victimDir) })

	victim := filepath.Join(victimDir, "precious.data")
	require.NoError(t, os.WriteFile(victim, []byte("do not delete"), 0o644))

	// Plant a malicious row pointing at the victim path.
	_, err = db.Exec(
		`INSERT INTO data_export_requests (user_id, status, file_path, ready_at, expires_at)
		 VALUES (?, 'ready', ?, NOW(), DATE_SUB(NOW(), INTERVAL 1 HOUR))`,
		userID, victim,
	)
	require.NoError(t, err)

	svc := privacy.NewService(db, nil, exportDir)
	n, err := svc.CleanupExpiredExports(context.Background())
	require.NoError(t, err)
	// The row IS marked expired (n==1) but the file MUST still exist.
	assert.Equal(t, int64(1), n)
	_, statErr := os.Stat(victim)
	assert.NoError(t, statErr,
		"CleanupExpiredExports must refuse to delete files outside the export dir")
}

func TestExport_ZipReadable(t *testing.T) {
	db := testutil.DBOrSkip(t)
	dir := setup(t)
	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"zr", "zr@t.l", "$2a$04$p", "ZR")
	var userID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='zr'`).Scan(&userID)

	svc := privacy.NewService(db, nil, dir)
	req, _ := svc.RequestExport(context.Background(), userID)
	require.NoError(t, svc.GenerateExport(context.Background(), req.ID))

	updated, _ := svc.GetActiveExport(context.Background(), userID)
	r, err := zip.OpenReader(updated.FilePath)
	require.NoError(t, err)
	defer r.Close()

	for _, f := range r.File {
		rc, err := f.Open()
		require.NoError(t, err)
		_, err = io.ReadAll(rc)
		require.NoError(t, err)
		rc.Close()
	}
}
