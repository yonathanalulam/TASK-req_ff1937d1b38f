package securitytest_test

// Deep attack coverage for the HMAC-signed /api/v1/internal/* routes.
//
// The verifier at internal/middleware/hmac_verify.go builds a signing message
// from METHOD + "\n" + PATH + "\n" + sha256(body). Each element is an
// independent lever an attacker could try to tamper with after observing a
// legitimate signed request. These tests probe every lever plus the header
// shape (scheme, hex format, key lookup) and document the current replay
// stance (no nonce/timestamp → replay is possible — see TestHMAC_Replay).

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const internalSourcesPath = "/api/v1/internal/data/sources"

// hmacSetup seeds an admin, logs them in, creates one key, and returns
// everything a test needs: the server, db, admin client + CSRF, key_id and
// secret. Keeps individual tests focused on the one attack they exercise.
func hmacSetup(t *testing.T, keyID string) (srv *httptest.Server, db *sql.DB, adminCli *http.Client, csrf string, secret []byte) {
	t.Helper()
	srv, db = securityServer(t)
	_ = seedUser(t, db, "adm_"+keyID, "administrator")
	adminCli = newClient(t, srv)
	csrf = loginAs(t, adminCli, srv.URL, "adm_"+keyID)
	secret = createHMACKey(t, adminCli, srv.URL, csrf, keyID)
	return
}

// ─── Header shape / format ───────────────────────────────────────────────────

func TestHMAC_MissingKeyID_Returns400(t *testing.T) {
	srv, _, _, _, secret := hmacSetup(t, "miss-kid")

	resp := doInternal(t, srv.Client(), srv.URL, http.MethodGet, internalSourcesPath,
		"miss-kid", secret, nil, internalOpts{SkipKeyID: true})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"missing X-Key-ID must return 400 hmac_missing")
}

func TestHMAC_MissingSignature_Returns400(t *testing.T) {
	srv, _, _, _, secret := hmacSetup(t, "miss-sig")

	resp := doInternal(t, srv.Client(), srv.URL, http.MethodGet, internalSourcesPath,
		"miss-sig", secret, nil, internalOpts{SkipSignature: true})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"missing X-Signature must return 400 hmac_missing")
}

func TestHMAC_WrongScheme_Returns401(t *testing.T) {
	// Scheme check is the first gate in appCrypto.Verify — sending a bearer
	// token masquerading as an HMAC signature must not slip through.
	srv, _, _, _, _ := hmacSetup(t, "scheme")

	resp := doInternal(t, srv.Client(), srv.URL, http.MethodGet, internalSourcesPath,
		"scheme", nil, nil, internalOpts{SigOverride: "bearer aaaaaaaaaaaa"})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"non-HMAC scheme must be rejected as hmac_invalid")
}

func TestHMAC_MalformedSignature_Returns401(t *testing.T) {
	// Header present but no space (no scheme/sig split) → Verify returns false.
	srv, _, _, _, _ := hmacSetup(t, "malform")

	resp := doInternal(t, srv.Client(), srv.URL, http.MethodGet, internalSourcesPath,
		"malform", nil, nil, internalOpts{SigOverride: "nospacehere"})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestHMAC_TruncatedHex_Returns401(t *testing.T) {
	// Half-length signature must fail — hmac.Equal is constant-time but still
	// returns false when byte slices have different lengths.
	srv, _, _, _, secret := hmacSetup(t, "trunc")

	realSig := signHMAC(http.MethodGet, internalSourcesPath, nil, secret)
	parts := strings.SplitN(realSig, " ", 2)
	truncated := parts[0] + " " + parts[1][:len(parts[1])/2]

	resp := doInternal(t, srv.Client(), srv.URL, http.MethodGet, internalSourcesPath,
		"trunc", nil, nil, internalOpts{SigOverride: truncated})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"truncated signatures must not authenticate")
}

// ─── Key lookup / discrimination ─────────────────────────────────────────────

func TestHMAC_UnknownKeyID_Returns401(t *testing.T) {
	// Unknown key_id should not be distinguishable from wrong-signature via
	// status code alone — both must return 401 so scanners can't enumerate
	// valid key_ids by probing for 404s.
	srv, _ := securityServer(t)
	resp := doInternal(t, srv.Client(), srv.URL, http.MethodGet, internalSourcesPath,
		"ghost-key", []byte("fake"), nil, internalOpts{})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"unknown key_id must surface as 401, not 404 — avoid key_id enumeration oracle")
}

func TestHMAC_RevokedKey_Returns401(t *testing.T) {
	srv, db, adminCli, csrf, secret := hmacSetup(t, "revokeme")

	// Sanity: active key authenticates.
	resp := doInternal(t, srv.Client(), srv.URL, http.MethodGet, internalSourcesPath,
		"revokeme", secret, nil, internalOpts{})
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "sanity: active key must authenticate")

	// Revoke via admin API.
	keyRowID := scanUintFromQuery(t, db, `SELECT id FROM hmac_keys WHERE key_id=?`, "revokeme")
	rev := doJSON(t, adminCli, http.MethodDelete,
		srv.URL+"/api/v1/admin/hmac-keys/"+u64Str(keyRowID), csrf, nil)
	rev.Body.Close()

	// Same secret, key now inactive → 401.
	resp = doInternal(t, srv.Client(), srv.URL, http.MethodGet, internalSourcesPath,
		"revokeme", secret, nil, internalOpts{})
	resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"revoked key must stop authenticating immediately")
}

func TestHMAC_CrossKeyForgery_Returns401(t *testing.T) {
	// Sign with key A's secret but send X-Key-ID=B. The server looks up B and
	// validates against B's secret, so the signature fails.
	srv, _, adminCli, csrf, secretA := hmacSetup(t, "key-a")
	_ = createHMACKey(t, adminCli, srv.URL, csrf, "key-b")

	sig := signHMAC(http.MethodGet, internalSourcesPath, nil, secretA)
	resp := doInternal(t, srv.Client(), srv.URL, http.MethodGet, internalSourcesPath,
		"key-b", nil, nil, internalOpts{SigOverride: sig})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"cross-key forgery: signature from A under header B must fail")
}

// ─── Message tampering ───────────────────────────────────────────────────────

func TestHMAC_BodyTamperingDetected(t *testing.T) {
	srv, _, _, _, secret := hmacSetup(t, "bodytamper")

	// Sign an empty body, but send a populated one.
	actualBody := []byte(`{"name":"pwned","source_type":"db_table","config":"{}"}`)
	resp := doInternal(t, srv.Client(), srv.URL, http.MethodPost, "/api/v1/internal/data/sources",
		"bodytamper", secret, actualBody,
		internalOpts{BodyForSign: []byte{}})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"body sha256 is part of the signed message — any change must invalidate the signature")
}

func TestHMAC_MethodTamperingDetected(t *testing.T) {
	srv, _, _, _, secret := hmacSetup(t, "methodtamper")

	// Signed as GET, sent as POST.
	wrongMethodSig := signHMAC(http.MethodGet, internalSourcesPath, nil, secret)
	resp := doInternal(t, srv.Client(), srv.URL, http.MethodPost, internalSourcesPath,
		"methodtamper", nil, []byte{}, internalOpts{SigOverride: wrongMethodSig})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"HTTP method is part of the signed message — swapping methods must fail")
}

func TestHMAC_PathTamperingDetected(t *testing.T) {
	srv, _, _, _, secret := hmacSetup(t, "pathtamper")

	// Signed for /jobs, sent to /sources.
	wrongPathSig := signHMAC(http.MethodGet, "/api/v1/internal/data/jobs", nil, secret)
	resp := doInternal(t, srv.Client(), srv.URL, http.MethodGet, internalSourcesPath,
		"pathtamper", nil, nil, internalOpts{SigOverride: wrongPathSig})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"path is part of the signed message — path swap must fail")
}

// ─── Replay (current-state documentation) ────────────────────────────────────

func TestHMAC_Replay_CurrentlyAccepted_Documented(t *testing.T) {
	// Documents the current behaviour: signatures do not include a nonce or
	// timestamp, so the same signed request can be replayed indefinitely
	// until the key is rotated. Captured as a test so that if replay
	// protection is ever added the assertion will flip and force a conscious
	// update here (and in hmac_verify.go).
	srv, _, _, _, secret := hmacSetup(t, "replayable")

	for i := 0; i < 2; i++ {
		resp := doInternal(t, srv.Client(), srv.URL, http.MethodGet, internalSourcesPath,
			"replayable", secret, nil, internalOpts{})
		resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode,
			"iteration %d: signature accepted — replay protection not implemented. "+
				"TODO(security): add timestamp + nonce to the signed message + a "+
				"bounded-TTL server-side nonce cache to close this window.", i+1)
	}
}

// ─── Verifier correctness under concurrent rotation ─────────────────────────

func TestHMAC_ConcurrentVerifyDuringRotation_NoServerErrors(t *testing.T) {
	// Fire a burst of signed reads while the admin rotates the key. Every
	// response must be 200 (pre-rotation) or 401 (post-rotation) — never a
	// 500 or panic. Guards against races in the verifier's load-and-decrypt
	// path.
	srv, _, adminCli, csrf, secret := hmacSetup(t, "racekey")

	var wg sync.WaitGroup
	const burst = 30
	statuses := make([]int, burst)
	for i := 0; i < burst; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			resp := doInternal(t, srv.Client(), srv.URL, http.MethodGet, internalSourcesPath,
				"racekey", secret, nil, internalOpts{})
			statuses[idx] = resp.StatusCode
			resp.Body.Close()
		}(i)
	}

	// Mid-burst: rotate.
	rot := doJSON(t, adminCli, http.MethodPost, srv.URL+"/api/v1/admin/hmac-keys/rotate",
		csrf, map[string]string{"key_id": "racekey"})
	rot.Body.Close()

	wg.Wait()

	// Invariant: no 5xx. Either 200 (before rotation visible) or 401 (after).
	for i, st := range statuses {
		assert.Contains(t, []int{http.StatusOK, http.StatusUnauthorized}, st,
			"burst request %d: unexpected status %d (want 200 or 401)", i, st)
	}
}
