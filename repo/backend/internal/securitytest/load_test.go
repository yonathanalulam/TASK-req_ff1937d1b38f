package securitytest_test

// Performance-under-security-load tests.
//
// These tests stress the security-critical paths that must remain correct
// under concurrency: account lockout counting, rate-limit bucket arithmetic,
// and HMAC key rotation vs. concurrent verifies. They are deliberately small
// and deterministic (small N, bounded goroutines) so they stay reliable in
// CI without turning into flakes.
//
// Performance here means correctness under load, not throughput benchmarks —
// the `go test -race` build catches data races, and these tests catch
// logical races (e.g. "five concurrent bad logins count as five failures,
// not fewer because the counter was last-write-wins").

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLockout_ConcurrentBadLogins_AllCounted checks that five simultaneous
// bad-password attempts all register as failures — the lockout counter must
// not lose updates under contention.
func TestLockout_ConcurrentBadLogins_AllCounted(t *testing.T) {
	srv, db := securityServer(t)
	_ = seedUser(t, db, "lockburst", "regular_user")

	var wg sync.WaitGroup
	const attempts = 5
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			body, _ := json.Marshal(map[string]string{
				"username": "lockburst",
				"password": "definitely-wrong",
			})
			resp, err := srv.Client().Post(srv.URL+"/api/v1/auth/login",
				"application/json", bytes.NewReader(body))
			if err == nil {
				resp.Body.Close()
			}
		}()
	}
	wg.Wait()

	// Verify five login_attempts rows exist for this user — the counter
	// drives the lockout threshold, so losing updates means losing lockout.
	var n int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM login_attempts WHERE username=? AND success=0`,
		"lockburst").Scan(&n)
	require.NoError(t, err)
	assert.Equal(t, attempts, n,
		"every concurrent bad-login attempt must be recorded; got %d of %d", n, attempts)

	// The 6th attempt (correct password) should surface as account_locked.
	body, _ := json.Marshal(map[string]string{"username": "lockburst", "password": "password"})
	resp, err := srv.Client().Post(srv.URL+"/api/v1/auth/login",
		"application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode,
		"after 5 failures the correct password must be blocked by lockout")
}

// TestRateLimit_ConcurrentBurst_EnforcesCap verifies the general rate
// limiter under concurrent login traffic: the observed count of 200/401
// (authenticated outcomes) must not exceed the 60/min bucket, regardless of
// how many goroutines try to push requests through.
func TestRateLimit_ConcurrentBurst_EnforcesCap(t *testing.T) {
	srv, db := securityServer(t)
	_ = seedUser(t, db, "burstuser", "regular_user")

	const total = 80 // exceeds the 60/min cap
	var (
		wg          sync.WaitGroup
		authOutcome int32 // 200 or 401 (not rate-limited)
		rateLimited int32 // 429
	)
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			body, _ := json.Marshal(map[string]string{
				"username": "burstuser",
				"password": "password",
			})
			resp, err := srv.Client().Post(srv.URL+"/api/v1/auth/login",
				"application/json", bytes.NewReader(body))
			if err != nil {
				return
			}
			resp.Body.Close()
			switch resp.StatusCode {
			case http.StatusOK, http.StatusUnauthorized, http.StatusForbidden:
				atomic.AddInt32(&authOutcome, 1)
			case http.StatusTooManyRequests:
				atomic.AddInt32(&rateLimited, 1)
			}
		}()
	}
	wg.Wait()

	// The auth outcomes cannot exceed the 60/min cap. Allow a small fudge
	// for the limiter's sliding-window granularity.
	const capPerMin = 60
	assert.LessOrEqual(t, int(authOutcome), capPerMin+5,
		"rate limiter must cap requests near %d/min (observed %d auth outcomes, %d rate-limited)",
		capPerMin, authOutcome, rateLimited)
	assert.Greater(t, int(rateLimited), 0,
		"with %d requests well above %d/min, at least some 429s must be returned",
		total, capPerMin)
}

// TestHMACRotation_ConcurrentRotations_NoCorruption ensures that two admins
// (or one admin clicking twice) racing a rotation never leaves the key row
// in an inconsistent state — at the end exactly one row exists for the
// key_id, it is active, and it has a fresh rotated_at.
//
// The shared adminCli + its cookie jar are safe for concurrent use by design
// (net/http.Client and cookiejar.Jar are both documented as goroutine-safe).
func TestHMACRotation_ConcurrentRotations_NoCorruption(t *testing.T) {
	srv, db, adminCli, csrf, _ := hmacSetup(t, "racerot")

	var wg sync.WaitGroup
	const racers = 5
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp := doJSON(t, adminCli, http.MethodPost,
				srv.URL+"/api/v1/admin/hmac-keys/rotate", csrf,
				map[string]string{"key_id": "racerot"})
			resp.Body.Close()
		}()
	}
	wg.Wait()

	// Exactly one row for racerot must remain.
	var n int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM hmac_keys WHERE key_id=?`, "racerot").Scan(&n))
	assert.Equal(t, 1, n,
		"concurrent rotation must not duplicate the key row (UNIQUE constraint + in-place UPDATE)")

	var isActive int
	var rotated sql.NullTime
	require.NoError(t, db.QueryRow(
		`SELECT is_active, rotated_at FROM hmac_keys WHERE key_id=?`, "racerot",
	).Scan(&isActive, &rotated))
	assert.Equal(t, 1, isActive, "post-rotation the key must be active")
	assert.True(t, rotated.Valid, "rotated_at must be set after at least one rotation")
}

// TestHMACRotateVsRevoke_RaceConvergesToOneOutcome pits rotate and revoke
// against each other. Either outcome is acceptable — what matters is that
// the row is in a consistent "all set" state at the end, not half-updated.
func TestHMACRotateVsRevoke_RaceConvergesToOneOutcome(t *testing.T) {
	srv, db, adminCli, csrf, _ := hmacSetup(t, "rvrace")
	keyRowID := scanUintFromQuery(t, db, `SELECT id FROM hmac_keys WHERE key_id=?`, "rvrace")

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		resp := doJSON(t, adminCli, http.MethodPost,
			srv.URL+"/api/v1/admin/hmac-keys/rotate", csrf,
			map[string]string{"key_id": "rvrace"})
		resp.Body.Close()
	}()
	go func() {
		defer wg.Done()
		resp := doJSON(t, adminCli, http.MethodDelete,
			srv.URL+"/api/v1/admin/hmac-keys/"+u64Str(keyRowID), csrf, nil)
		resp.Body.Close()
	}()
	wg.Wait()

	// The row must exist and is_active must be exactly 0 or 1 — no torn writes.
	var isActive int
	var rotated sql.NullTime
	err := db.QueryRow(
		`SELECT is_active, rotated_at FROM hmac_keys WHERE key_id=?`, "rvrace",
	).Scan(&isActive, &rotated)
	require.NoError(t, err, "row must still exist after race")
	assert.Contains(t, []int{0, 1}, isActive,
		"is_active must be exactly 0 or 1 (no torn writes)")
}
