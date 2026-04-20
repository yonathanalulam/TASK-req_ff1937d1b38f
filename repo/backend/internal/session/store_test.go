package session_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eagle-point/service-portal/internal/models"
	"github.com/eagle-point/service-portal/internal/session"
	"github.com/eagle-point/service-portal/internal/testutil"
)

// ─── Unit: Session model methods ─────────────────────────────────────────────

func TestSession_IsExpired_True(t *testing.T) {
	s := models.Session{ExpiresAt: time.Now().Add(-1 * time.Second)}
	assert.True(t, s.IsExpired())
}

func TestSession_IsExpired_False(t *testing.T) {
	s := models.Session{ExpiresAt: time.Now().Add(time.Hour)}
	assert.False(t, s.IsExpired())
}

func TestSession_IsInactive_True(t *testing.T) {
	s := models.Session{LastActiveAt: time.Now().Add(-31 * time.Minute)}
	assert.True(t, s.IsInactive(30*time.Minute))
}

func TestSession_IsInactive_False(t *testing.T) {
	s := models.Session{LastActiveAt: time.Now().Add(-5 * time.Minute)}
	assert.False(t, s.IsInactive(30*time.Minute))
}

// ─── Integration: Store CRUD ──────────────────────────────────────────────────

func TestStore_CreateAndGet(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db, "sessions", "user_roles", "user_preferences", "login_attempts", "users")

	_, err := db.Exec(
		`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"sessuser", "sessuser@test.local", "$2a$04$placeholder", "Sess User",
	)
	require.NoError(t, err)

	var userID uint64
	require.NoError(t, db.QueryRow(`SELECT id FROM users WHERE username='sessuser'`).Scan(&userID))

	store := session.New(db)
	sess, err := store.Create(context.Background(), userID, "127.0.0.1", "test-agent")
	require.NoError(t, err)
	require.NotNil(t, sess)

	assert.NotEmpty(t, sess.ID)
	assert.NotEmpty(t, sess.CSRFToken)
	assert.Equal(t, userID, sess.UserID)
	assert.False(t, sess.IsExpired())
	assert.False(t, sess.IsInactive(session.InactivityTimeout))

	got, err := store.GetByID(context.Background(), sess.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, sess.ID, got.ID)
	assert.Equal(t, sess.CSRFToken, got.CSRFToken)
}

func TestStore_GetByID_NotFound(t *testing.T) {
	db := testutil.DBOrSkip(t)
	store := session.New(db)

	got, err := store.GetByID(context.Background(), "nonexistent-session-id-xyz")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestStore_Delete(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db, "sessions", "user_roles", "user_preferences", "login_attempts", "users")

	_, _ = db.Exec(
		`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"deluser", "del@test.local", "$2a$04$placeholder", "Del",
	)
	var userID uint64
	_ = db.QueryRow(`SELECT id FROM users WHERE username='deluser'`).Scan(&userID)

	store := session.New(db)
	sess, err := store.Create(context.Background(), userID, "127.0.0.1", "ua")
	require.NoError(t, err)

	require.NoError(t, store.Delete(context.Background(), sess.ID))

	got, err := store.GetByID(context.Background(), sess.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestStore_Touch_UpdatesLastActive(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db, "sessions", "user_roles", "user_preferences", "login_attempts", "users")

	_, _ = db.Exec(
		`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"touchuser", "touch@test.local", "$2a$04$placeholder", "Touch",
	)
	var userID uint64
	_ = db.QueryRow(`SELECT id FROM users WHERE username='touchuser'`).Scan(&userID)

	store := session.New(db)
	sess, _ := store.Create(context.Background(), userID, "127.0.0.1", "ua")

	// Small sleep to ensure last_active_at changes
	time.Sleep(10 * time.Millisecond)
	require.NoError(t, store.Touch(context.Background(), sess.ID))

	refreshed, err := store.GetByID(context.Background(), sess.ID)
	require.NoError(t, err)
	assert.True(t, refreshed.LastActiveAt.After(sess.LastActiveAt))
}

func TestStore_DeleteAllForUser(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db, "sessions", "user_roles", "user_preferences", "login_attempts", "users")

	_, _ = db.Exec(
		`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"multiuser", "multi@test.local", "$2a$04$placeholder", "Multi",
	)
	var userID uint64
	_ = db.QueryRow(`SELECT id FROM users WHERE username='multiuser'`).Scan(&userID)

	store := session.New(db)
	// Create 3 sessions for same user
	for i := 0; i < 3; i++ {
		_, _ = store.Create(context.Background(), userID, "127.0.0.1", "ua")
	}

	require.NoError(t, store.DeleteAllForUser(context.Background(), userID))

	// Verify all gone
	var count int
	_ = db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id=?`, userID).Scan(&count)
	assert.Equal(t, 0, count)
}
