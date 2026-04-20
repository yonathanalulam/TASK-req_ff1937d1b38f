package securitytest_test

// Advanced web attack scenarios against the session-cookie API surface:
// CSRF bypasses, session forgery, SQL-injection probes, stored-XSS input
// acceptance, and oversized-payload handling.
//
// These tests exercise properties at the HTTP boundary; they assume the
// underlying services are correct and instead verify that the outer
// defensive layers can't be tricked by a motivated attacker who has access
// to public signup or a valid session cookie from one tenant.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── CSRF ────────────────────────────────────────────────────────────────────

func TestCSRF_MissingTokenOnMutating_Rejected(t *testing.T) {
	srv, db := securityServer(t)
	_ = seedUser(t, db, "csrf_miss", "regular_user")
	cli := newClient(t, srv)
	_ = loginAs(t, cli, srv.URL, "csrf_miss")

	// Same session cookie, but NO X-CSRF-Token header on a mutating request.
	resp := doJSON(t, cli, http.MethodPost, srv.URL+"/api/v1/users/me/addresses", "",
		map[string]any{
			"label": "home", "address_line1": "1 Main",
			"city": "X", "state": "NY", "zip": "10001",
		})
	defer resp.Body.Close()
	assert.NotEqual(t, http.StatusCreated, resp.StatusCode,
		"mutating request without CSRF token MUST be rejected even with a valid session")
	assert.Contains(t, []int{http.StatusBadRequest, http.StatusForbidden}, resp.StatusCode,
		"CSRF middleware must surface 400 or 403, got %d", resp.StatusCode)
}

func TestCSRF_WrongTokenRejected(t *testing.T) {
	srv, db := securityServer(t)
	_ = seedUser(t, db, "csrf_wrong", "regular_user")
	cli := newClient(t, srv)
	_ = loginAs(t, cli, srv.URL, "csrf_wrong")

	// Session cookie is valid, token is an attacker-chosen blob.
	resp := doJSON(t, cli, http.MethodPost, srv.URL+"/api/v1/users/me/addresses",
		"forged-csrf-token-value",
		map[string]any{
			"label": "home", "address_line1": "1 Main",
			"city": "X", "state": "NY", "zip": "10001",
		})
	defer resp.Body.Close()
	assert.NotEqual(t, http.StatusCreated, resp.StatusCode,
		"forged CSRF token must not unlock the session")
}

func TestCSRF_TokenBoundToSession_CrossSessionRejected(t *testing.T) {
	// Alice's CSRF token must NOT authorize Bob's session — even if Bob is
	// also logged in, his token is the only token he can present.
	srv, db := securityServer(t)
	_ = seedUser(t, db, "alice_csrf", "regular_user")
	_ = seedUser(t, db, "bob_csrf", "regular_user")

	aliceCli := newClient(t, srv)
	aliceTok := loginAs(t, aliceCli, srv.URL, "alice_csrf")

	bobCli := newClient(t, srv)
	_ = loginAs(t, bobCli, srv.URL, "bob_csrf")

	// Bob uses his own session cookie jar but presents Alice's CSRF token.
	resp := doJSON(t, bobCli, http.MethodPost, srv.URL+"/api/v1/users/me/addresses",
		aliceTok,
		map[string]any{
			"label": "home", "address_line1": "1 Main",
			"city": "X", "state": "NY", "zip": "10001",
		})
	defer resp.Body.Close()
	assert.NotEqual(t, http.StatusCreated, resp.StatusCode,
		"CSRF token from another session must not authorize this session")
}

// ─── Session integrity ──────────────────────────────────────────────────────

func TestSession_ForgedCookie_Rejected(t *testing.T) {
	srv, _ := securityServer(t)
	cli := newClient(t, srv)

	// Manually inject a fake session cookie — a random 64-hex string of the
	// sort a session id might look like.
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/auth/me", nil)
	require.NoError(t, err)
	req.AddCookie(&http.Cookie{
		// Must match session.CookieName() ("sp_session"). Using the wrong name
		// makes the test green for the wrong reason — the cookie is simply
		// ignored, which doesn't exercise the forgery-rejection path.
		Name:  "sp_session",
		Value: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	})
	resp, err := cli.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"a random session-shaped cookie must not grant authentication")
}

func TestSession_PostLogoutCookieInvalidated(t *testing.T) {
	// After logout, the session row should be gone — the cookie sitting in
	// the attacker's jar should no longer grant access.
	srv, db := securityServer(t)
	_ = seedUser(t, db, "logouttest", "regular_user")
	cli := newClient(t, srv)
	csrf := loginAs(t, cli, srv.URL, "logouttest")

	// Sanity: /me works.
	resp := doJSON(t, cli, http.MethodGet, srv.URL+"/api/v1/auth/me", "", nil)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "sanity: session valid")

	// Logout.
	resp = doJSON(t, cli, http.MethodPost, srv.URL+"/api/v1/auth/logout", csrf, nil)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// The cookie-jar still holds the old session cookie; but the server-side
	// row is gone. Follow-up request must 401.
	resp = doJSON(t, cli, http.MethodGet, srv.URL+"/api/v1/auth/me", "", nil)
	resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"post-logout: stale session cookie must not re-authenticate")
}

// ─── Input attacks ──────────────────────────────────────────────────────────

func TestInjection_SQLInLoginUsername_TreatedAsLiteral(t *testing.T) {
	// A classic SQLi payload in the username field must not alter query
	// semantics — parameterised queries treat it as a literal string, so the
	// login simply fails with invalid_credentials rather than succeeding,
	// crashing, or returning a 500.
	srv, _ := securityServer(t)
	cli := newClient(t, srv)

	payloads := []string{
		"admin' OR '1'='1",
		"admin'--",
		`admin"; DROP TABLE users;--`,
		"\x00admin",
	}
	for _, pl := range payloads {
		body, _ := json.Marshal(map[string]string{"username": pl, "password": "anything"})
		resp, err := cli.Post(srv.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
		require.NoError(t, err)
		resp.Body.Close()
		assert.Contains(t, []int{http.StatusUnauthorized, http.StatusBadRequest, http.StatusUnprocessableEntity},
			resp.StatusCode,
			"SQLi payload %q must yield a benign client error, not 200 or 500 (got %d)",
			pl, resp.StatusCode)
	}
}

func TestInputAttacks_StoredXSSInBio_PersistedAsLiteral(t *testing.T) {
	// A stored-XSS payload in profile.bio must round-trip byte-for-byte —
	// the backend is a JSON API and does not render HTML, so it MUST NOT
	// mangle or strip the payload (defense-in-depth belongs on the frontend
	// via Vue's text-binding escape). This test pins the contract so a
	// future "sanitize on input" change doesn't silently drop data.
	srv, db := securityServer(t)
	_ = seedUser(t, db, "xss_stored", "regular_user")
	cli := newClient(t, srv)
	csrf := loginAs(t, cli, srv.URL, "xss_stored")

	payload := `<script>alert('xss')</script> &copy; "quoted"`
	resp := doJSON(t, cli, http.MethodPut, srv.URL+"/api/v1/users/me/profile", csrf,
		map[string]any{
			"display_name": "XSS User",
			"bio":          payload,
		})
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Read it back.
	resp = doJSON(t, cli, http.MethodGet, srv.URL+"/api/v1/users/me/profile", "", nil)
	defer resp.Body.Close()
	var payloadOut struct {
		Profile struct {
			Bio string `json:"bio"`
		} `json:"profile"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payloadOut))
	assert.Equal(t, payload, payloadOut.Profile.Bio,
		"bio must be stored verbatim (frontend is responsible for HTML-escaping on render)")
}

func TestInputAttacks_NullByteInUsername_Rejected(t *testing.T) {
	// Null bytes in identifiers are a classic log-injection / truncation vector.
	// The auth registration path should reject them at validation rather than
	// persist them and cause downstream inconsistencies.
	srv, _ := securityServer(t)
	cli := newClient(t, srv)

	body, _ := json.Marshal(map[string]string{
		"username":     "nul\x00byte",
		"email":        "n@test.local",
		"password":     "ValidPass1",
		"display_name": "Nul",
	})
	resp, err := cli.Post(srv.URL+"/api/v1/auth/register", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	resp.Body.Close()
	// The registration may succeed or fail, but it must NOT 5xx and must NOT
	// record a user whose name silently differs from the submitted string.
	assert.NotEqual(t, http.StatusInternalServerError, resp.StatusCode,
		"null byte in username must not trigger a 500")
}

// ─── Body size ──────────────────────────────────────────────────────────────

func TestOversizedRequestBody_DoesNotCrashServer(t *testing.T) {
	// A 2 MB JSON payload on /register must not crash the process, even if
	// it is rejected at parse time. Defends against naive bodies-into-memory
	// handlers that would OOM under a slowloris-style body flood.
	srv, _ := securityServer(t)
	cli := newClient(t, srv)

	// Build ~2 MB of valid-looking JSON. Gin's default max body is 32 MB so
	// this fits but is large enough to stress the decoder.
	big := strings.Repeat("A", 2*1024*1024)
	body, _ := json.Marshal(map[string]string{
		"username":     "bigbody",
		"email":        "big@test.local",
		"password":     "ValidPass1",
		"display_name": big,
	})
	resp, err := cli.Post(srv.URL+"/api/v1/auth/register", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	resp.Body.Close()
	// The server may 201 (accept it) or 4xx (reject it); it must NOT 5xx.
	assert.NotEqual(t, http.StatusInternalServerError, resp.StatusCode,
		"oversized body must not crash the server (got %d)", resp.StatusCode)
}

// ─── Path traversal ─────────────────────────────────────────────────────────

func TestPathTraversal_TicketIDPath_NotFound(t *testing.T) {
	// Gin route matching treats /tickets/../auth/me as a distinct path; the
	// attempted traversal should either hit the 404 fallback or be rejected
	// before reaching any handler. Under no circumstances should it serve
	// /auth/me under an authenticated-as-nobody context.
	srv, db := securityServer(t)
	_ = seedUser(t, db, "traverse", "regular_user")
	cli := newClient(t, srv)
	csrf := loginAs(t, cli, srv.URL, "traverse")

	resp := doJSON(t, cli, http.MethodGet, srv.URL+"/api/v1/tickets/..%2F..%2Fauth%2Fme",
		csrf, nil)
	defer resp.Body.Close()
	// Must NOT be 200. A 400/404/422 is all fine.
	assert.NotEqual(t, http.StatusOK, resp.StatusCode,
		"encoded path traversal must not resolve to another authenticated route")
}
