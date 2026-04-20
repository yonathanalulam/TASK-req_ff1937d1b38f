package auth_test

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eagle-point/service-portal/internal/auth"
	"github.com/eagle-point/service-portal/internal/config"
	appCrypto "github.com/eagle-point/service-portal/internal/crypto"
	"github.com/eagle-point/service-portal/internal/router"
	"github.com/eagle-point/service-portal/internal/testutil"
)

// integrationServer returns a test httptest.Server backed by a real database.
func integrationServer(t *testing.T) *httptest.Server {
	t.Helper()
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"login_attempts", "sessions", "user_roles", "user_preferences", "users",
	)

	cfg := &config.Config{
		AppEnv:              "test",
		Port:                "8080",
		DBHost:              "db",
		DBPort:              "3306",
		FieldEncryptionKey:  "",
		SessionCookieDomain: "",
	}

	r := router.New(cfg, db)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

// doJSON is a helper for making JSON requests.
func doJSON(t *testing.T, client *http.Client, method, url string, body any, headers map[string]string) *http.Response {
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
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	require.NoError(t, err)
	return resp
}

// ─── Full register → login → protected access → logout ────────────────────────

func TestIntegration_RegisterLoginLogout(t *testing.T) {
	srv := integrationServer(t)
	client := srv.Client()
	// Use cookie jar to persist session
	jar := newCookieJar()
	client.Jar = jar

	base := srv.URL

	// 1. Register
	resp := doJSON(t, client, http.MethodPost, base+"/api/v1/auth/register", map[string]string{
		"username":     "intuser",
		"email":        "int@example.local",
		"password":     "ValidPass1",
		"display_name": "Int User",
	}, nil)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// 2. Login
	resp = doJSON(t, client, http.MethodPost, base+"/api/v1/auth/login", map[string]string{
		"username": "intuser",
		"password": "ValidPass1",
	}, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Session cookie must be HttpOnly in every request. Secure is asserted
	// in TestIntegration_SessionCookie_SecureWhenTLS where we simulate a
	// TLS-terminated request; httptest.NewServer here runs plain HTTP and a
	// Secure cookie would be rejected by stdlib cookiejar, breaking the
	// rest of this flow test.
	assertSessionCookieHttpOnly(t, resp, "login")

	var loginBody map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&loginBody)
	resp.Body.Close()

	csrfToken, _ := loginBody["csrf_token"].(string)
	require.NotEmpty(t, csrfToken)

	// 3. GET /me — should succeed (cookie is set)
	resp = doJSON(t, client, http.MethodGet, base+"/api/v1/auth/me", nil, nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// 4. Logout
	resp = doJSON(t, client, http.MethodPost, base+"/api/v1/auth/logout", nil,
		map[string]string{"X-CSRF-Token": csrfToken})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assertSessionCookieHttpOnly(t, resp, "logout")
	resp.Body.Close()

	// 5. GET /me after logout — should be 401
	resp = doJSON(t, client, http.MethodGet, base+"/api/v1/auth/me", nil, nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()
}

// ─── RBAC enforcement ─────────────────────────────────────────────────────────

func TestIntegration_RBAC_AdminRouteBlocksRegularUser(t *testing.T) {
	srv := integrationServer(t)
	client := srv.Client()
	client.Jar = newCookieJar()
	base := srv.URL

	// Register as regular user (default role)
	doJSON(t, client, http.MethodPost, base+"/api/v1/auth/register", map[string]string{
		"username": "rbacuser", "email": "rbac@example.local",
		"password": "ValidPass1", "display_name": "RBAC",
	}, nil).Body.Close()

	resp := doJSON(t, client, http.MethodPost, base+"/api/v1/auth/login", map[string]string{
		"username": "rbacuser", "password": "ValidPass1",
	}, nil)
	var loginBody map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&loginBody)
	resp.Body.Close()
	csrf := loginBody["csrf_token"].(string)

	// Hit admin-only endpoint
	resp = doJSON(t, client, http.MethodPost, base+"/api/v1/admin/hmac-keys/rotate", nil,
		map[string]string{"X-CSRF-Token": csrf})
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	resp.Body.Close()
}

// ─── Account lockout ──────────────────────────────────────────────────────────

func TestIntegration_Lockout_After5BadPasswords(t *testing.T) {
	srv := integrationServer(t)
	client := srv.Client()
	client.Jar = newCookieJar()
	base := srv.URL

	doJSON(t, client, http.MethodPost, base+"/api/v1/auth/register", map[string]string{
		"username": "lockuser", "email": "lock@example.local",
		"password": "ValidPass1", "display_name": "Lock",
	}, nil).Body.Close()

	// 5 bad password attempts
	for i := 0; i < 5; i++ {
		resp := doJSON(t, client, http.MethodPost, base+"/api/v1/auth/login", map[string]string{
			"username": "lockuser", "password": "WrongPass9",
		}, nil)
		resp.Body.Close()
	}

	// 6th attempt (correct password) — should be locked
	resp := doJSON(t, client, http.MethodPost, base+"/api/v1/auth/login", map[string]string{
		"username": "lockuser", "password": "ValidPass1",
	}, nil)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	errObj, _ := body["error"].(map[string]interface{})
	assert.Equal(t, "account_locked", errObj["code"])
}

// ─── Rate limiting ────────────────────────────────────────────────────────────

func TestIntegration_RateLimit_Returns429(t *testing.T) {
	srv := integrationServer(t)
	client := srv.Client()
	client.Jar = newCookieJar()
	base := srv.URL

	// Hit the register endpoint 61 times from the same IP
	// The general rate limiter (60/min) should trigger on the 61st
	var lastCode int
	for i := 0; i < 65; i++ {
		resp := doJSON(t, client, http.MethodPost, base+"/api/v1/auth/register", map[string]string{
			"username": "rlu" + string(rune('a'+i%26)),
			"email":    "rlu" + string(rune('a'+i%26)) + "@x.local",
			"password": "ValidPass1", "display_name": "RL",
		}, nil)
		lastCode = resp.StatusCode
		resp.Body.Close()
		if lastCode == http.StatusTooManyRequests {
			break
		}
	}
	assert.Equal(t, http.StatusTooManyRequests, lastCode)
}

// ─── HMAC — missing / wrong signature ────────────────────────────────────────
//
// These tests hit a real HMAC-protected internal route (/api/v1/internal/data/sources)
// with deterministic statuses: 400 for missing headers, 401 for invalid
// signature, 200 for a seeded valid signature. Previously these accepted "404
// or 4xx" which would have masked the route being accidentally unregistered.

const internalSourcesPath = "/api/v1/internal/data/sources"

// hmacIntegrationServer spins up a server with a usable encryption key so we
// can seed an hmac_keys row whose secret is encrypted the way the verifier
// expects to decrypt it. The default integrationServer uses an empty key,
// which would make loadSecret fail before we can exercise the signature path.
func hmacIntegrationServer(t *testing.T) (*httptest.Server, *sql.DB, string) {
	t.Helper()
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"hmac_keys",
		"login_attempts", "sessions", "user_roles", "user_preferences", "users",
	)

	const encKey = "0000000000000000000000000000000000000000000000000000000000000000"
	cfg := &config.Config{
		AppEnv:              "test",
		Port:                "8080",
		DBHost:              "db",
		DBPort:              "3306",
		FieldEncryptionKey:  encKey,
		SessionCookieDomain: "",
	}
	r := router.New(cfg, db)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, db, encKey
}

// seedHMACKey inserts a known secret for keyID and returns the raw secret
// bytes so the caller can sign requests deterministically.
func seedHMACKey(t *testing.T, db *sql.DB, encKey, keyID string) []byte {
	t.Helper()
	secret := []byte("integration-test-secret-32-bytes")
	encrypted, err := appCrypto.Encrypt(secret, encKey)
	require.NoError(t, err)
	_, err = db.Exec(
		`INSERT INTO hmac_keys (key_id, secret_encrypted, is_active) VALUES (?, ?, 1)`,
		keyID, encrypted)
	require.NoError(t, err)
	return secret
}

// signHMAC mirrors middleware/hmac_verify.go's buildHMACMessage.
func signHMAC(method, path string, body, secret []byte) string {
	h := sha256.Sum256(body)
	msg := method + "\n" + path + "\n" + hex.EncodeToString(h[:])
	return "hmac-sha256 " + appCrypto.Sign(msg, secret)
}

func TestIntegration_HMAC_MissingHeaders_Returns400(t *testing.T) {
	srv, _, _ := hmacIntegrationServer(t)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+internalSourcesPath, nil)
	resp, err := srv.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Exact-status assertion: the HMAC middleware's first gate rejects
	// header-less requests with 400 hmac_missing. Anything else (including
	// 404) indicates the internal route/middleware is unregistered.
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestIntegration_HMAC_WrongSignature_Returns401(t *testing.T) {
	srv, db, encKey := hmacIntegrationServer(t)
	_ = seedHMACKey(t, db, encKey, "wrong-sig-key")

	req, _ := http.NewRequest(http.MethodGet, srv.URL+internalSourcesPath, nil)
	req.Header.Set("X-Key-ID", "wrong-sig-key")
	// Well-formed header (scheme + hex of right length) but signed with the
	// wrong message → Verify returns false and the verifier aborts with 401.
	req.Header.Set("X-Signature",
		"hmac-sha256 0000000000000000000000000000000000000000000000000000000000000000")
	resp, err := srv.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestIntegration_HMAC_ValidSignature_Returns200(t *testing.T) {
	srv, db, encKey := hmacIntegrationServer(t)
	secret := seedHMACKey(t, db, encKey, "valid-key")

	req, _ := http.NewRequest(http.MethodGet, srv.URL+internalSourcesPath, nil)
	req.Header.Set("X-Key-ID", "valid-key")
	req.Header.Set("X-Signature",
		signHMAC(http.MethodGet, internalSourcesPath, nil, secret))

	resp, err := srv.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// A correctly-signed read against the ingest sources list endpoint must
	// reach the handler and return 200 — no 404, no 401. This is the
	// counterpart to the negative cases above and proves the whole signing
	// pipeline works end-to-end against a real route.
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// ─── Protected route requires auth ───────────────────────────────────────────

func TestIntegration_UnauthenticatedRequest_Returns401(t *testing.T) {
	srv := integrationServer(t)
	client := srv.Client()
	base := srv.URL

	resp := doJSON(t, client, http.MethodGet, base+"/api/v1/auth/me", nil, nil)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// ─── Cookie jar helper ────────────────────────────────────────────────────────

type simpleCookieJar struct {
	cookies map[string][]*http.Cookie
}

func newCookieJar() *simpleCookieJar {
	return &simpleCookieJar{cookies: make(map[string][]*http.Cookie)}
}

func (j *simpleCookieJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.cookies[u.Host] = append(j.cookies[u.Host], cookies...)
}

func (j *simpleCookieJar) Cookies(u *url.URL) []*http.Cookie {
	return j.cookies[u.Host]
}

// assertSessionCookieHttpOnly checks HttpOnly only. Secure is transport-
// dependent (see isSecureRequest in handler.go) and asserted separately by
// TestIntegration_SessionCookie_SecureWhenTLS which forces a TLS-terminated
// request context.
func assertSessionCookieHttpOnly(t *testing.T, resp *http.Response, step string) {
	t.Helper()
	var cookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "sp_session" {
			cookie = c
			break
		}
	}
	require.NotNilf(t, cookie, "%s: sp_session cookie missing in response", step)
	assert.Truef(t, cookie.HttpOnly, "%s: sp_session cookie must set HttpOnly=true", step)
}

// TestIntegration_SessionCookie_SecureWhenTLS verifies that when the request
// arrives over HTTPS (simulated via X-Forwarded-Proto for plain-HTTP httptest
// servers), the session cookie is emitted with Secure=true. This is the
// positive half of the cookie-hardening assertion — the production posture
// is "Secure whenever TLS is actually terminated upstream," not "Secure gated
// by APP_ENV."
func TestIntegration_SessionCookie_SecureWhenTLS(t *testing.T) {
	srv := integrationServer(t)
	base := srv.URL

	registerResp := doJSON(t, srv.Client(), http.MethodPost, base+"/api/v1/auth/register", map[string]string{
		"username":     "secureuser",
		"email":        "secure@example.local",
		"password":     "ValidPass1",
		"display_name": "Secure User",
	}, nil)
	require.Equal(t, http.StatusCreated, registerResp.StatusCode)
	registerResp.Body.Close()

	// Build the login request manually so we can set X-Forwarded-Proto to
	// simulate a TLS-terminated request (as nginx/any reverse proxy would).
	loginBody, _ := json.Marshal(map[string]string{
		"username": "secureuser", "password": "ValidPass1",
	})
	req, err := http.NewRequest(http.MethodPost, base+"/api/v1/auth/login", bytes.NewReader(loginBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-Proto", "https")

	resp, err := srv.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var cookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "sp_session" {
			cookie = c
			break
		}
	}
	require.NotNil(t, cookie, "sp_session cookie missing")
	assert.True(t, cookie.Secure, "sp_session must set Secure=true when the request arrives over TLS")
	assert.True(t, cookie.HttpOnly, "sp_session must always set HttpOnly=true")
}

// Ensure compile: auth package referenced for ErrInvalidCredentials
var _ = auth.ErrInvalidCredentials
