// Package securitytest holds integration-level tests that exercise security
// properties at the router boundary: authorization (IDOR), HMAC attack
// surface, CSRF/session forgery, and concurrency correctness of rate-limit +
// lockout + HMAC rotation.
//
// Tests here are deliberately isolated from individual domain packages so
// security coverage stays grep-able as a distinct discipline, and so a single
// shared set of fixtures (cookie jars, login helpers, HMAC signers) can be
// reused across all four test files.
package securitytest_test

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eagle-point/service-portal/internal/config"
	appCrypto "github.com/eagle-point/service-portal/internal/crypto"
	"github.com/eagle-point/service-portal/internal/router"
	"github.com/eagle-point/service-portal/internal/testutil"
)

// Shared test encryption key (matches docker-compose test default). Using a
// non-empty key exercises the AES-GCM code paths that the middleware
// actually runs in production, so tests don't silently skip decryption logic.
const testEncKey = "0000000000000000000000000000000000000000000000000000000000000000"

// bcrypt hash of "password" — reused by every seeded user. The seed and test
// passwords are intentionally shared across this package.
const seedPwHash = "$2a$10$92IXUNpkjO0rOQ5byMi.Ye4oKoEa3Ro9llC/.og/at2.uheWG/igi"

// allTables is the superset of tables we truncate at test start. A single
// list keeps fixtures in sync across the four test files without each one
// needing to know which tables its particular scenarios touch.
var allTables = []string{
	"hmac_keys",
	"audit_logs",
	"notification_outbox", "notifications", "notification_templates",
	"violation_records", "moderation_actions", "moderation_queue", "sensitive_terms",
	"qa_posts", "qa_threads",
	"review_reports", "review_images", "reviews",
	"ticket_attachments", "ticket_notes", "tickets",
	"service_offerings", "service_categories",
	"shipping_templates", "shipping_regions",
	"addresses",
	"data_export_requests", "data_deletion_requests",
	"ingest_checkpoints", "ingest_jobs", "ingest_sources",
	"lakehouse_lineage", "lakehouse_metadata",
	"lakehouse_schema_versions", "lakehouse_lifecycle_policies", "legal_holds",
	"login_attempts", "sessions", "user_roles", "user_preferences", "user_favorites", "user_browsing_history", "users",
}

// securityServer builds a fresh httptest server bound to a truncated DB.
// APP_ENV=test keeps the dev HMAC bootstrap from firing so tests control the
// hmac_keys table contents.
func securityServer(t *testing.T) (*httptest.Server, *sql.DB) {
	t.Helper()
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db, allTables...)

	cfg := &config.Config{
		AppEnv:              "test",
		Port:                "8080",
		DBHost:              "db",
		DBPort:              "3306",
		FieldEncryptionKey:  testEncKey,
		SessionCookieDomain: "localhost",
	}
	r := router.New(cfg, db)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, db
}

// seedUser inserts a user, assigns the requested role, and returns its id.
// Password is always "password". Panics if the role doesn't exist in `roles`.
func seedUser(t *testing.T, db *sql.DB, username, role string) uint64 {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		username, username+"@sec.test", seedPwHash, username)
	require.NoError(t, err)

	var uid uint64
	require.NoError(t, db.QueryRow(
		`SELECT id FROM users WHERE username=?`, username).Scan(&uid))

	_, err = db.Exec(
		`INSERT INTO user_roles (user_id, role_id) SELECT ?, id FROM roles WHERE name=?`,
		uid, role)
	require.NoError(t, err)

	_, _ = db.Exec(
		`INSERT INTO user_preferences (user_id, notify_in_app) VALUES (?, 1)`, uid)
	return uid
}

// newClient returns an http.Client backed by a fresh cookie jar. Multiple
// clients per server simulate isolated users — each has its own session.
func newClient(t *testing.T, srv *httptest.Server) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	c := srv.Client()
	c.Jar = jar
	return c
}

// loginAs logs the given username in on the given client and returns the
// CSRF token the server handed back. Fails the test on any non-200 response.
func loginAs(t *testing.T, client *http.Client, base, username string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": "password"})
	resp, err := client.Post(base+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "login must succeed for seeded user %q", username)

	var payload map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	csrf, _ := payload["csrf_token"].(string)
	require.NotEmpty(t, csrf)
	return csrf
}

// doJSON issues a JSON request with the CSRF header set (if non-empty).
// Does NOT add auth — the caller's cookie jar carries the session cookie.
func doJSON(t *testing.T, client *http.Client, method, url, csrf string, body any) *http.Response {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, url, reader)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	resp, err := client.Do(req)
	require.NoError(t, err)
	return resp
}

// readBody drains and returns the response body as a string, closing it.
// Useful when asserting error envelope contents.
func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(b)
}

// ─── HMAC helpers ────────────────────────────────────────────────────────────

// createHMACKey uses the admin API to mint a key and returns (keyID, secret).
// Assumes the client is logged in as an administrator.
func createHMACKey(t *testing.T, client *http.Client, base, csrf, keyID string) []byte {
	t.Helper()
	resp := doJSON(t, client, http.MethodPost, base+"/api/v1/admin/hmac-keys",
		csrf, map[string]string{"key_id": keyID})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var payload map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	resp.Body.Close()
	secretHex, _ := payload["secret"].(string)
	require.NotEmpty(t, secretHex)
	b, err := hex.DecodeString(secretHex)
	require.NoError(t, err)
	return b
}

// signHMAC produces the X-Signature header value matching the verifier's
// message format: METHOD + "\n" + PATH + "\n" + sha256_hex(body).
func signHMAC(method, path string, body, secret []byte) string {
	h := sha256.Sum256(body)
	msg := method + "\n" + path + "\n" + hex.EncodeToString(h[:])
	return "hmac-sha256 " + appCrypto.Sign(msg, secret)
}

// doInternal issues a request to an /api/v1/internal/* route, signing it
// with the given key. Use helper overrides (sigOverride, keyIDOverride,
// bodyOverride) to test tampering scenarios.
type internalOpts struct {
	SigOverride   string // non-empty → used instead of computed signature
	KeyIDOverride string // non-empty → used instead of canonical keyID
	SkipKeyID     bool   // true → omit X-Key-ID entirely
	SkipSignature bool   // true → omit X-Signature entirely
	BodyForSign   []byte // if non-nil, sign this instead of the actual body (tampering)
}

func doInternal(t *testing.T, client *http.Client, base, method, path, keyID string, secret, body []byte, opts internalOpts) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, base+path, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	if !opts.SkipKeyID {
		headerKeyID := keyID
		if opts.KeyIDOverride != "" {
			headerKeyID = opts.KeyIDOverride
		}
		req.Header.Set("X-Key-ID", headerKeyID)
	}
	if !opts.SkipSignature {
		sig := opts.SigOverride
		if sig == "" {
			bodyForSign := body
			if opts.BodyForSign != nil {
				bodyForSign = opts.BodyForSign
			}
			sig = signHMAC(method, path, bodyForSign, secret)
		}
		req.Header.Set("X-Signature", sig)
	}
	resp, err := client.Do(req)
	require.NoError(t, err)
	return resp
}

// ─── Small utilities ─────────────────────────────────────────────────────────

// scanUintFromQuery is a compact helper for "SELECT id FROM x WHERE y=z"
// style lookups used when seed data needs its primary key read back.
func scanUintFromQuery(t *testing.T, db *sql.DB, q string, args ...any) uint64 {
	t.Helper()
	var id uint64
	require.NoError(t, db.QueryRow(q, args...).Scan(&id))
	return id
}

// u64Str returns the decimal representation of u as a string — for composing
// URLs like /tickets/{id}. Avoids pulling strconv into every test.
func u64Str(u uint64) string { return strconv.FormatUint(u, 10) }
