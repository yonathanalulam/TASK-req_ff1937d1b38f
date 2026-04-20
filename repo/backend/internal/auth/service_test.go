package auth_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/eagle-point/service-portal/internal/auth"
	"github.com/eagle-point/service-portal/internal/session"
	"github.com/eagle-point/service-portal/internal/testutil"
)

// ─── Unit: password hashing ───────────────────────────────────────────────────

func TestBcrypt_HashAndVerify(t *testing.T) {
	password := "ValidPass1"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 4) // cost 4 for test speed
	require.NoError(t, err)

	assert.NoError(t, bcrypt.CompareHashAndPassword(hash, []byte(password)))
	assert.Error(t, bcrypt.CompareHashAndPassword(hash, []byte("wrongpassword")))
}

func TestBcrypt_DifferentHashEachCall(t *testing.T) {
	pw := []byte("ValidPass1")
	h1, _ := bcrypt.GenerateFromPassword(pw, 4)
	h2, _ := bcrypt.GenerateFromPassword(pw, 4)
	// bcrypt salts are random — hashes must differ
	assert.NotEqual(t, string(h1), string(h2))
}

// ─── Integration: register + login ───────────────────────────────────────────

func TestService_Register_Success(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"login_attempts", "sessions", "user_roles", "user_preferences", "users")

	ss := session.New(db)
	svc := auth.NewService(db, ss)

	user, err := svc.Register(context.Background(), auth.RegisterInput{
		Username:    "testuser",
		Email:       "test@example.local",
		Password:    "ValidPass1",
		DisplayName: "Test User",
	})
	require.NoError(t, err)
	assert.NotZero(t, user.ID)
	assert.Equal(t, "testuser", user.Username)
	assert.Contains(t, user.Roles, "regular_user")
}

func TestService_Register_DuplicateUsername(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"login_attempts", "sessions", "user_roles", "user_preferences", "users")

	ss := session.New(db)
	svc := auth.NewService(db, ss)

	input := auth.RegisterInput{
		Username:    "dupuser",
		Email:       "dup@example.local",
		Password:    "ValidPass1",
		DisplayName: "Dup",
	}
	_, err := svc.Register(context.Background(), input)
	require.NoError(t, err)

	// Second registration with same username
	input.Email = "other@example.local"
	_, err = svc.Register(context.Background(), input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already taken")
}

func TestService_Login_ValidCredentials(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"login_attempts", "sessions", "user_roles", "user_preferences", "users")

	ss := session.New(db)
	svc := auth.NewService(db, ss)

	_, err := svc.Register(context.Background(), auth.RegisterInput{
		Username:    "logintest",
		Email:       "login@example.local",
		Password:    "ValidPass1",
		DisplayName: "Login Test",
	})
	require.NoError(t, err)

	out, err := svc.Login(context.Background(), auth.LoginInput{
		Username:  "logintest",
		Password:  "ValidPass1",
		IPAddress: "127.0.0.1",
		UserAgent: "test-agent",
	})
	require.NoError(t, err)
	assert.Equal(t, "logintest", out.User.Username)
	assert.NotEmpty(t, out.Session.ID)
	assert.NotEmpty(t, out.Session.CSRFToken)
}

func TestService_Login_InvalidPassword(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"login_attempts", "sessions", "user_roles", "user_preferences", "users")

	ss := session.New(db)
	svc := auth.NewService(db, ss)

	_, _ = svc.Register(context.Background(), auth.RegisterInput{
		Username:    "wrongpw",
		Email:       "wp@example.local",
		Password:    "ValidPass1",
		DisplayName: "WP",
	})

	_, err := svc.Login(context.Background(), auth.LoginInput{
		Username:  "wrongpw",
		Password:  "BadPassword9",
		IPAddress: "127.0.0.1",
	})
	assert.ErrorIs(t, err, auth.ErrInvalidCredentials)
}

// ─── Integration: account lockout ─────────────────────────────────────────────

func TestService_Lockout_After5Failures(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"login_attempts", "sessions", "user_roles", "user_preferences", "users")

	ss := session.New(db)
	svc := auth.NewService(db, ss)

	_, _ = svc.Register(context.Background(), auth.RegisterInput{
		Username:    "lockme",
		Email:       "lock@example.local",
		Password:    "ValidPass1",
		DisplayName: "Lock Me",
	})

	// 5 consecutive bad passwords
	for i := 0; i < 5; i++ {
		_, err := svc.Login(context.Background(), auth.LoginInput{
			Username:  "lockme",
			Password:  "WrongPass9",
			IPAddress: "127.0.0.1",
		})
		require.Error(t, err)
	}

	// 6th attempt — should be locked regardless of password correctness
	_, err := svc.Login(context.Background(), auth.LoginInput{
		Username:  "lockme",
		Password:  "ValidPass1", // correct password — still blocked
		IPAddress: "127.0.0.1",
	})
	require.Error(t, err)

	var lockErr *auth.LockoutError
	assert.ErrorAs(t, err, &lockErr)
	assert.Greater(t, lockErr.RemainingSeconds(), 0)
}
