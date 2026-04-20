package hmacadmin_test

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eagle-point/service-portal/internal/config"
	appCrypto "github.com/eagle-point/service-portal/internal/crypto"
	"github.com/eagle-point/service-portal/internal/router"
	"github.com/eagle-point/service-portal/internal/testutil"
)

// ─── fixtures ────────────────────────────────────────────────────────────────

// bcrypt hash of "password" (shared test password used across the repo).
const seedPwHashHMAC = "$2a$10$92IXUNpkjO0rOQ5byMi.Ye4oKoEa3Ro9llC/.og/at2.uheWG/igi"

// hmacAdminHTTP spins up the full router backed by a truncated DB and returns
// an httptest server. The caller is expected to TruncateTables for anything
// outside the narrow set this fixture resets.
func hmacAdminHTTP(t *testing.T) (*httptest.Server, *sql.DB) {
	t.Helper()
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"hmac_keys",
		"audit_logs",
		"login_attempts", "sessions", "user_roles", "user_preferences", "users",
		"ingest_checkpoints", "ingest_jobs", "ingest_sources",
	)
	cfg := &config.Config{
		AppEnv: "test", Port: "8080", DBHost: "db", DBPort: "3306",
		FieldEncryptionKey:  testEncKey,
		SessionCookieDomain: "localhost",
	}
	r := router.New(cfg, db)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, db
}

// seedAdmin inserts a user with the administrator role and returns the
// username. Password is always "password".
func seedAdmin(t *testing.T, db *sql.DB, username string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		username, username+"@test.local", seedPwHashHMAC, username)
	require.NoError(t, err)
	var uid uint64
	require.NoError(t, db.QueryRow(`SELECT id FROM users WHERE username=?`, username).Scan(&uid))
	_, err = db.Exec(`INSERT INTO user_roles (user_id, role_id) SELECT ?, id FROM roles WHERE name=?`,
		uid, "administrator")
	require.NoError(t, err)
	_, _ = db.Exec(`INSERT INTO user_preferences (user_id, notify_in_app) VALUES (?, 1)`, uid)
}

// loginAsAdmin logs in via the auth endpoint and returns the CSRF token the
// server hands back. The provided client must have a cookie jar so the
// session cookie persists.
func loginAsAdmin(t *testing.T, client *http.Client, base, username string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": "password"})
	resp, err := client.Post(base+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "login must succeed for seeded admin")

	var payload map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	csrf, _ := payload["csrf_token"].(string)
	require.NotEmpty(t, csrf)
	return csrf
}

// doJSON issues a JSON request with the CSRF header set.
func doJSON(t *testing.T, client *http.Client, method, url, csrf string, body any) *http.Response {
	t.Helper()
	var buf *bytes.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		buf = bytes.NewReader(raw)
	} else {
		buf = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, url, buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	resp, err := client.Do(req)
	require.NoError(t, err)
	return resp
}

// signInternalRequest builds the HMAC signature the verifier expects:
//   HMAC-SHA256(secret, METHOD + "\n" + PATH + "\n" + sha256_hex(body))
func signInternalRequest(method, path string, body, secret []byte) string {
	h := sha256.Sum256(body)
	msg := method + "\n" + path + "\n" + hex.EncodeToString(h[:])
	return "hmac-sha256 " + appCrypto.Sign(msg, secret)
}

// jarClient returns an httptest.Server client with a cookie jar already
// attached.
func jarClient(t *testing.T, srv *httptest.Server) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	c := srv.Client()
	c.Jar = jar
	return c
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestHTTP_CreateKey_RevealsSecretOnce(t *testing.T) {
	srv, db := hmacAdminHTTP(t)
	seedAdmin(t, db, "adm_create")
	client := jarClient(t, srv)
	csrf := loginAsAdmin(t, client, srv.URL, "adm_create")

	resp := doJSON(t, client, http.MethodPost, srv.URL+"/api/v1/admin/hmac-keys",
		csrf, map[string]string{"key_id": "reporting-worker"})
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var payload map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))

	secret, _ := payload["secret"].(string)
	assert.NotEmpty(t, secret, "create response must reveal the plaintext secret")
	assert.Len(t, secret, 64, "secret must be 32 bytes hex-encoded")

	warn, _ := payload["secret_reveal_warning"].(string)
	assert.Contains(t, warn, "NOT be shown again",
		"response must make the one-shot semantics visible to the caller")
}

func TestHTTP_CreateKey_RequiresAdmin(t *testing.T) {
	srv, db := hmacAdminHTTP(t)
	// Seed a non-admin user and attempt the operation.
	_, err := db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"regular_hm", "reg@test.local", seedPwHashHMAC, "regular")
	require.NoError(t, err)
	var uid uint64
	db.QueryRow(`SELECT id FROM users WHERE username=?`, "regular_hm").Scan(&uid)
	_, _ = db.Exec(`INSERT INTO user_roles (user_id, role_id) SELECT ?, id FROM roles WHERE name=?`,
		uid, "regular_user")
	_, _ = db.Exec(`INSERT INTO user_preferences (user_id, notify_in_app) VALUES (?, 1)`, uid)

	client := jarClient(t, srv)
	csrf := loginAsAdmin(t, client, srv.URL, "regular_hm")

	resp := doJSON(t, client, http.MethodPost, srv.URL+"/api/v1/admin/hmac-keys",
		csrf, map[string]string{"key_id": "should-fail"})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode,
		"non-admin must be blocked by the RBAC middleware")
}

func TestHTTP_RotateKey_OldSecretStopsWorking(t *testing.T) {
	// End-to-end proof that rotation is a real hard cut-over: after rotation
	// the original secret must be rejected by the HMAC verifier on
	// /api/v1/internal routes, and the new secret must be accepted.
	srv, db := hmacAdminHTTP(t)
	seedAdmin(t, db, "adm_rot")
	client := jarClient(t, srv)
	csrf := loginAsAdmin(t, client, srv.URL, "adm_rot")

	// 1) Create a fresh key and capture its secret.
	resp := doJSON(t, client, http.MethodPost, srv.URL+"/api/v1/admin/hmac-keys",
		csrf, map[string]string{"key_id": "ingest-bot"})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var created map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	resp.Body.Close()
	oldSecretHex, _ := created["secret"].(string)
	oldSecret, err := hex.DecodeString(oldSecretHex)
	require.NoError(t, err)

	// 2) Confirm the old secret currently works against /internal/data/sources.
	internalPath := "/api/v1/internal/data/sources"
	req, _ := http.NewRequest(http.MethodGet, srv.URL+internalPath, nil)
	req.Header.Set("X-Key-ID", "ingest-bot")
	req.Header.Set("X-Signature", signInternalRequest("GET", internalPath, nil, oldSecret))
	// Internal routes don't require the admin cookie; use a vanilla client.
	resp, err = srv.Client().Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode,
		"pre-rotation: the freshly-revealed secret must authenticate")

	// 3) Rotate.
	resp = doJSON(t, client, http.MethodPost, srv.URL+"/api/v1/admin/hmac-keys/rotate",
		csrf, map[string]string{"key_id": "ingest-bot"})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var rotated map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&rotated))
	resp.Body.Close()
	newSecretHex, _ := rotated["secret"].(string)
	assert.NotEqual(t, oldSecretHex, newSecretHex, "rotation must produce a different secret")
	newSecret, err := hex.DecodeString(newSecretHex)
	require.NoError(t, err)

	// 4) Old secret now fails.
	req, _ = http.NewRequest(http.MethodGet, srv.URL+internalPath, nil)
	req.Header.Set("X-Key-ID", "ingest-bot")
	req.Header.Set("X-Signature", signInternalRequest("GET", internalPath, nil, oldSecret))
	resp, err = srv.Client().Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"post-rotation: old secret must be rejected")

	// 5) New secret succeeds.
	req, _ = http.NewRequest(http.MethodGet, srv.URL+internalPath, nil)
	req.Header.Set("X-Key-ID", "ingest-bot")
	req.Header.Set("X-Signature", signInternalRequest("GET", internalPath, nil, newSecret))
	resp, err = srv.Client().Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"post-rotation: new secret must authenticate")
}

func TestHTTP_RevokeKey_StopsWorking(t *testing.T) {
	srv, db := hmacAdminHTTP(t)
	seedAdmin(t, db, "adm_rev")
	client := jarClient(t, srv)
	csrf := loginAsAdmin(t, client, srv.URL, "adm_rev")

	// Create a key.
	resp := doJSON(t, client, http.MethodPost, srv.URL+"/api/v1/admin/hmac-keys",
		csrf, map[string]string{"key_id": "shortlived"})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var created map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	resp.Body.Close()
	secretHex, _ := created["secret"].(string)
	secret, _ := hex.DecodeString(secretHex)
	keyInfo, _ := created["key"].(map[string]any)
	keyRowID := int64(keyInfo["id"].(float64))

	// Revoke it.
	resp = doJSON(t, client, http.MethodDelete,
		srv.URL+"/api/v1/admin/hmac-keys/"+intToStr(keyRowID), csrf, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Verifier now treats the key as non-existent (is_active=0 filtered out).
	internalPath := "/api/v1/internal/data/sources"
	req, _ := http.NewRequest(http.MethodGet, srv.URL+internalPath, nil)
	req.Header.Set("X-Key-ID", "shortlived")
	req.Header.Set("X-Signature", signInternalRequest("GET", internalPath, nil, secret))
	resp, err := srv.Client().Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"revoked keys must no longer authenticate")
}

func TestHTTP_ListKeys_ExcludesSecrets(t *testing.T) {
	srv, db := hmacAdminHTTP(t)
	seedAdmin(t, db, "adm_list")
	client := jarClient(t, srv)
	csrf := loginAsAdmin(t, client, srv.URL, "adm_list")

	// Seed two keys directly so List has something to return.
	for _, kid := range []string{"one", "two"} {
		resp := doJSON(t, client, http.MethodPost, srv.URL+"/api/v1/admin/hmac-keys",
			csrf, map[string]string{"key_id": kid})
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		resp.Body.Close()
	}

	resp := doJSON(t, client, http.MethodGet, srv.URL+"/api/v1/admin/hmac-keys", csrf, nil)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var payload struct {
		Keys []map[string]any `json:"keys"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	require.Len(t, payload.Keys, 2)
	for _, k := range payload.Keys {
		// The list projection deliberately excludes secrets.
		_, hasSecret := k["secret"]
		_, hasEncrypted := k["secret_encrypted"]
		assert.False(t, hasSecret, "list must not leak plaintext secrets")
		assert.False(t, hasEncrypted, "list must not leak ciphertext blobs either")
		assert.NotEmpty(t, k["key_id"])
	}
}

func TestHTTP_CreateKey_DuplicateReturnsConflict(t *testing.T) {
	srv, db := hmacAdminHTTP(t)
	seedAdmin(t, db, "adm_dup")
	client := jarClient(t, srv)
	csrf := loginAsAdmin(t, client, srv.URL, "adm_dup")

	resp := doJSON(t, client, http.MethodPost, srv.URL+"/api/v1/admin/hmac-keys",
		csrf, map[string]string{"key_id": "taken"})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	resp = doJSON(t, client, http.MethodPost, srv.URL+"/api/v1/admin/hmac-keys",
		csrf, map[string]string{"key_id": "taken"})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestHTTP_RotateUnknownKey_Returns404(t *testing.T) {
	srv, db := hmacAdminHTTP(t)
	seedAdmin(t, db, "adm_404")
	client := jarClient(t, srv)
	csrf := loginAsAdmin(t, client, srv.URL, "adm_404")

	resp := doJSON(t, client, http.MethodPost, srv.URL+"/api/v1/admin/hmac-keys/rotate",
		csrf, map[string]string{"key_id": "ghost"})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func intToStr(n int64) string {
	// Tiny stdlib-free int-to-string to avoid dragging strconv into the test
	// helper surface; sufficient for positive IDs.
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
