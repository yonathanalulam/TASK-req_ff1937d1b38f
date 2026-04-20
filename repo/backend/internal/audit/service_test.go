package audit_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eagle-point/service-portal/internal/audit"
	"github.com/eagle-point/service-portal/internal/models"
	"github.com/eagle-point/service-portal/internal/testutil"
)

func setup(t *testing.T) {
	t.Helper()
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"audit_logs",
		"data_export_requests", "data_deletion_requests",
		"login_attempts", "sessions", "user_roles", "user_preferences", "users",
	)
}

func TestWrite_AndList(t *testing.T) {
	db := testutil.DBOrSkip(t)
	setup(t)
	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"audituser", "au@t.l", "$2a$04$p", "Au")
	var userID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='audituser'`).Scan(&userID)

	svc := audit.NewService(db)
	require.NoError(t, svc.Write(context.Background(), audit.Entry{
		UserID: &userID,
		Action: models.AuditActionLogin,
		IPAddress: "127.0.0.1",
		Metadata: map[string]interface{}{"reason": "test"},
	}))

	logs, err := svc.List(context.Background(), userID, 10)
	require.NoError(t, err)
	require.Len(t, logs, 1)
	assert.Equal(t, models.AuditActionLogin, logs[0].Action)
	assert.Equal(t, "127.0.0.1", logs[0].IPAddress)
	require.NotNil(t, logs[0].Metadata)
	assert.Equal(t, "test", logs[0].Metadata["reason"])
}

func TestWrite_RequiresAction(t *testing.T) {
	db := testutil.DBOrSkip(t)
	svc := audit.NewService(db)
	err := svc.Write(context.Background(), audit.Entry{Action: ""})
	assert.Error(t, err)
}

func TestList_FiltersByUser(t *testing.T) {
	db := testutil.DBOrSkip(t)
	setup(t)
	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?),(?,?,?,?)`,
		"u_a", "ua@t.l", "$2a$04$p", "UA",
		"u_b", "ub@t.l", "$2a$04$p", "UB")
	var aID, bID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='u_a'`).Scan(&aID)
	db.QueryRow(`SELECT id FROM users WHERE username='u_b'`).Scan(&bID)

	svc := audit.NewService(db)
	svc.Write(context.Background(), audit.Entry{UserID: &aID, Action: "x"})
	svc.Write(context.Background(), audit.Entry{UserID: &bID, Action: "y"})

	aLogs, _ := svc.List(context.Background(), aID, 10)
	assert.Len(t, aLogs, 1)
	assert.Equal(t, "x", aLogs[0].Action)

	all, _ := svc.List(context.Background(), 0, 10)
	assert.Len(t, all, 2)
}

func TestPurgeBefore(t *testing.T) {
	db := testutil.DBOrSkip(t)
	setup(t)
	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"purger", "pg@t.l", "$2a$04$p", "P")
	var userID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='purger'`).Scan(&userID)

	svc := audit.NewService(db)
	svc.Write(context.Background(), audit.Entry{UserID: &userID, Action: "old"})
	// Force the row's created_at into the past
	db.Exec(`UPDATE audit_logs SET created_at = '2010-01-01 00:00:00'`)

	cutoff := time.Now().Add(-30 * 24 * time.Hour).Format("2006-01-02 15:04:05")
	n, err := svc.PurgeBefore(context.Background(), cutoff)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
}
