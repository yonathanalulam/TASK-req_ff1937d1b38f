package profile_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eagle-point/service-portal/internal/profile"
	"github.com/eagle-point/service-portal/internal/testutil"
)

// ─── Integration: profile update ─────────────────────────────────────────────

func TestProfile_UpdateAndGet(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"user_browsing_history", "user_favorites", "user_preferences",
		"login_attempts", "sessions", "user_roles", "users",
	)

	_, err := db.Exec(
		`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"profuser", "prof@test.local", "$2a$04$placeholder", "Orig Name",
	)
	require.NoError(t, err)
	var userID uint64
	require.NoError(t, db.QueryRow(`SELECT id FROM users WHERE username='profuser'`).Scan(&userID))

	svc := profile.NewService(db, "") // empty key = test mode (no encryption)

	// Update profile (non-admin caller → phone masked on return)
	updated, err := svc.UpdateProfile(context.Background(), userID, profile.UpdateProfileInput{
		DisplayName: "New Name",
		Bio:         "Hello world",
		Phone:       "4155551234",
	})
	require.NoError(t, err)
	assert.Equal(t, "New Name", updated.DisplayName)
	assert.Equal(t, "Hello world", updated.Bio)
	// Phone must be masked for non-admin reads
	assert.Equal(t, "(415) ***-1234", updated.Phone)
}

func TestProfile_UpdateProfile_EmptyDisplayName_Fails(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"user_browsing_history", "user_favorites", "user_preferences",
		"login_attempts", "sessions", "user_roles", "users",
	)

	_, _ = db.Exec(
		`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"emptyname", "en@test.local", "$2a$04$placeholder", "Valid",
	)
	var userID uint64
	_ = db.QueryRow(`SELECT id FROM users WHERE username='emptyname'`).Scan(&userID)

	svc := profile.NewService(db, "")
	_, err := svc.UpdateProfile(context.Background(), userID, profile.UpdateProfileInput{
		DisplayName: "",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "display_name")
}

// ─── Integration: preferences ────────────────────────────────────────────────

func TestPreferences_UpdateAndGet(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"user_browsing_history", "user_favorites", "user_preferences",
		"login_attempts", "sessions", "user_roles", "users",
	)

	_, _ = db.Exec(
		`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"prefuser", "pref@test.local", "$2a$04$placeholder", "Pref User",
	)
	var userID uint64
	_ = db.QueryRow(`SELECT id FROM users WHERE username='prefuser'`).Scan(&userID)

	svc := profile.NewService(db, "")

	prefs, err := svc.UpdatePreferences(context.Background(), userID, profile.UpdatePreferencesInput{
		NotifyInApp:  false,
		MutedTags:    []int64{1, 2, 3},
		MutedAuthors: []int64{10},
	})
	require.NoError(t, err)
	assert.False(t, prefs.NotifyInApp)
	assert.Equal(t, []int64{1, 2, 3}, prefs.MutedTags)
	assert.Equal(t, []int64{10}, prefs.MutedAuthors)

	// Read back
	got, err := svc.GetPreferences(context.Background(), userID)
	require.NoError(t, err)
	assert.False(t, got.NotifyInApp)
	assert.Equal(t, []int64{1, 2, 3}, got.MutedTags)
}

// ─── Integration: favorites ──────────────────────────────────────────────────

func TestFavorites_AddListRemove(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"user_browsing_history", "user_favorites", "user_preferences",
		"login_attempts", "sessions", "user_roles", "users",
	)

	_, _ = db.Exec(
		`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"favuser", "fav@test.local", "$2a$04$placeholder", "Fav User",
	)
	var userID uint64
	_ = db.QueryRow(`SELECT id FROM users WHERE username='favuser'`).Scan(&userID)

	svc := profile.NewService(db, "")

	require.NoError(t, svc.AddFavorite(context.Background(), userID, 42))
	require.NoError(t, svc.AddFavorite(context.Background(), userID, 43))

	// Idempotent add
	require.NoError(t, svc.AddFavorite(context.Background(), userID, 42))

	page, err := svc.ListFavorites(context.Background(), userID, 0, 20)
	require.NoError(t, err)
	assert.Len(t, page.Items, 2)

	require.NoError(t, svc.RemoveFavorite(context.Background(), userID, 42))

	page, err = svc.ListFavorites(context.Background(), userID, 0, 20)
	require.NoError(t, err)
	assert.Len(t, page.Items, 1)
	assert.Equal(t, uint64(43), page.Items[0].OfferingID)
}

// ─── Integration: browsing history ───────────────────────────────────────────

func TestHistory_RecordAndClear(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"user_browsing_history", "user_favorites", "user_preferences",
		"login_attempts", "sessions", "user_roles", "users",
	)

	_, _ = db.Exec(
		`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"histuser", "hist@test.local", "$2a$04$placeholder", "Hist User",
	)
	var userID uint64
	_ = db.QueryRow(`SELECT id FROM users WHERE username='histuser'`).Scan(&userID)

	svc := profile.NewService(db, "")

	require.NoError(t, svc.RecordView(context.Background(), userID, 100))
	require.NoError(t, svc.RecordView(context.Background(), userID, 101))

	page, err := svc.ListHistory(context.Background(), userID, 0, 20)
	require.NoError(t, err)
	assert.Len(t, page.Items, 2)

	require.NoError(t, svc.ClearHistory(context.Background(), userID))

	page, err = svc.ListHistory(context.Background(), userID, 0, 20)
	require.NoError(t, err)
	assert.Len(t, page.Items, 0)
}

// ─── Integration: favorites pagination ───────────────────────────────────────

func TestFavorites_CursorPagination(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"user_browsing_history", "user_favorites", "user_preferences",
		"login_attempts", "sessions", "user_roles", "users",
	)

	_, _ = db.Exec(
		`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"pageuser", "page@test.local", "$2a$04$placeholder", "Page",
	)
	var userID uint64
	_ = db.QueryRow(`SELECT id FROM users WHERE username='pageuser'`).Scan(&userID)

	svc := profile.NewService(db, "")
	for i := uint64(1); i <= 5; i++ {
		require.NoError(t, svc.AddFavorite(context.Background(), userID, i))
	}

	// Fetch first page of 3
	page1, err := svc.ListFavorites(context.Background(), userID, 0, 3)
	require.NoError(t, err)
	assert.Len(t, page1.Items, 3)
	assert.NotZero(t, page1.NextCursor)

	// Fetch next page using cursor
	page2, err := svc.ListFavorites(context.Background(), userID, page1.NextCursor, 3)
	require.NoError(t, err)
	assert.Len(t, page2.Items, 2)
	assert.Zero(t, page2.NextCursor) // no more pages
}
