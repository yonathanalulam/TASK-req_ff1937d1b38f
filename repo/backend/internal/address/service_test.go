package address_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eagle-point/service-portal/internal/address"
	"github.com/eagle-point/service-portal/internal/testutil"
)

// ─── Unit: ZIP validation ─────────────────────────────────────────────────────

func TestValidateZip_Valid5Digit(t *testing.T) {
	assert.True(t, address.ValidateZip("12345"))
}

func TestValidateZip_ValidZip4(t *testing.T) {
	assert.True(t, address.ValidateZip("12345-6789"))
}

func TestValidateZip_InvalidLetters(t *testing.T) {
	assert.False(t, address.ValidateZip("ABCDE"))
}

func TestValidateZip_TooShort(t *testing.T) {
	assert.False(t, address.ValidateZip("1234"))
}

func TestValidateZip_TooLong(t *testing.T) {
	assert.False(t, address.ValidateZip("123456"))
}

func TestValidateZip_BadDash(t *testing.T) {
	assert.False(t, address.ValidateZip("12345-123"))  // only 3 digits after dash
}

func TestValidateZip_Empty(t *testing.T) {
	assert.False(t, address.ValidateZip(""))
}

// ─── Integration: address CRUD ───────────────────────────────────────────────

func TestAddress_CreateAndList(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db, "addresses", "user_roles", "user_preferences", "login_attempts", "sessions", "users")

	_, err := db.Exec(
		`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"addruser", "addr@test.local", "$2a$04$placeholder", "Addr User",
	)
	require.NoError(t, err)

	var userID uint64
	require.NoError(t, db.QueryRow(`SELECT id FROM users WHERE username='addruser'`).Scan(&userID))

	svc := address.NewService(db, "")

	addr, err := svc.Create(context.Background(), userID, address.CreateInput{
		Label:        "Home",
		AddressLine1: "123 Main St",
		City:         "Springfield",
		State:        "IL",
		Zip:          "62701",
	})
	require.NoError(t, err)
	require.NotNil(t, addr)
	assert.Equal(t, "123 Main St", addr.AddressLine1)
	assert.Equal(t, "Springfield", addr.City)
	assert.Equal(t, "IL", addr.State)
	assert.Equal(t, "62701", addr.Zip)
	assert.True(t, addr.IsDefault, "first address should auto-become default")

	list, err := svc.List(context.Background(), userID)
	require.NoError(t, err)
	assert.Len(t, list, 1)
}

func TestAddress_InvalidZip_ReturnsError(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db, "addresses", "user_roles", "user_preferences", "login_attempts", "sessions", "users")

	_, _ = db.Exec(
		`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"zipfail", "zipfail@test.local", "$2a$04$placeholder", "Zip Fail",
	)
	var userID uint64
	_ = db.QueryRow(`SELECT id FROM users WHERE username='zipfail'`).Scan(&userID)

	svc := address.NewService(db, "")
	_, err := svc.Create(context.Background(), userID, address.CreateInput{
		Label: "Home", AddressLine1: "1 Main", City: "City", State: "CA", Zip: "ABCDE",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ZIP")
}

func TestAddress_SetDefault_ClearsPrevious(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db, "addresses", "user_roles", "user_preferences", "login_attempts", "sessions", "users")

	_, _ = db.Exec(
		`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"defuser", "def@test.local", "$2a$04$placeholder", "Def User",
	)
	var userID uint64
	_ = db.QueryRow(`SELECT id FROM users WHERE username='defuser'`).Scan(&userID)

	svc := address.NewService(db, "")

	addr1, err := svc.Create(context.Background(), userID, address.CreateInput{
		Label: "Home", AddressLine1: "1 Home St", City: "A", State: "CA", Zip: "90210",
	})
	require.NoError(t, err)
	assert.True(t, addr1.IsDefault)

	addr2, err := svc.Create(context.Background(), userID, address.CreateInput{
		Label: "Work", AddressLine1: "2 Work Ave", City: "B", State: "NY", Zip: "10001",
	})
	require.NoError(t, err)
	assert.False(t, addr2.IsDefault)

	// Promote addr2 as default
	updated, err := svc.SetDefault(context.Background(), userID, addr2.ID)
	require.NoError(t, err)
	assert.True(t, updated.IsDefault)

	// Re-fetch list — addr1 should no longer be default
	list, err := svc.List(context.Background(), userID)
	require.NoError(t, err)
	require.Len(t, list, 2)

	defaultCount := 0
	for _, a := range list {
		if a.IsDefault {
			defaultCount++
			assert.Equal(t, addr2.ID, a.ID, "only addr2 should be default")
		}
	}
	assert.Equal(t, 1, defaultCount, "exactly one address must be default")
}

func TestAddress_Delete(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db, "addresses", "user_roles", "user_preferences", "login_attempts", "sessions", "users")

	_, _ = db.Exec(
		`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"deladdr", "deladdr@test.local", "$2a$04$placeholder", "Del Addr",
	)
	var userID uint64
	_ = db.QueryRow(`SELECT id FROM users WHERE username='deladdr'`).Scan(&userID)

	svc := address.NewService(db, "")
	addr, err := svc.Create(context.Background(), userID, address.CreateInput{
		Label: "Home", AddressLine1: "5 Oak Ln", City: "C", State: "TX", Zip: "75001",
	})
	require.NoError(t, err)

	require.NoError(t, svc.Delete(context.Background(), userID, addr.ID))

	list, err := svc.List(context.Background(), userID)
	require.NoError(t, err)
	assert.Len(t, list, 0)
}

func TestAddress_DeleteNotOwned_ReturnsNotFound(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db, "addresses", "user_roles", "user_preferences", "login_attempts", "sessions", "users")

	svc := address.NewService(db, "")
	err := svc.Delete(context.Background(), 9999, 9999)
	require.Error(t, err)
	assert.ErrorIs(t, err, address.ErrNotFound)
}
