package ingest_test

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eagle-point/service-portal/internal/config"
	"github.com/eagle-point/service-portal/internal/crypto"
	"github.com/eagle-point/service-portal/internal/router"
	"github.com/eagle-point/service-portal/internal/testutil"
)

// Test-mode encryption key (matches docker-compose test FIELD_ENCRYPTION_KEY).
const testEncKey = "0000000000000000000000000000000000000000000000000000000000000000"

func ingestServer(t *testing.T) (*httptest.Server, *sql.DB) {
	t.Helper()
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"hmac_keys",
		"lakehouse_lineage", "lakehouse_metadata",
		"lakehouse_schema_versions", "lakehouse_lifecycle_policies", "legal_holds",
		"ingest_checkpoints", "ingest_jobs", "ingest_sources",
		"audit_logs",
		"data_export_requests", "data_deletion_requests",
		"login_attempts", "sessions", "user_roles", "user_preferences", "users",
	)
	cfg := &config.Config{
		AppEnv: "test", Port: "8080", DBHost: "db", DBPort: "3306",
		// HMAC verifier needs a real key to decrypt secrets
		FieldEncryptionKey:  testEncKey,
		SessionCookieDomain: "localhost",
	}
	r := router.New(cfg, db)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, db
}

// seedHMACKey inserts an HMAC key row and returns (keyID, secret).
func seedHMACKey(t *testing.T, db *sql.DB) (string, []byte) {
	t.Helper()
	keyID := "test-key"
	secret := []byte("super-secret-bytes")
	enc, err := crypto.Encrypt(secret, testEncKey)
	require.NoError(t, err)
	db.Exec(`INSERT INTO hmac_keys (key_id, secret_encrypted, is_active) VALUES (?, ?, 1)`,
		keyID, enc)
	return keyID, secret
}

// signHMAC builds the HMAC signature header value as the verifier expects.
func signHMAC(method, path string, body []byte, secret []byte) string {
	bodyHash := sha256.Sum256(body)
	msg := method + "\n" + path + "\n" + hex.EncodeToString(bodyHash[:])
	sig := crypto.Sign(msg, secret)
	return "hmac-sha256 " + sig
}

func doInternal(t *testing.T, client *http.Client, srvURL, method, path string, body []byte, keyID string, secret []byte) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, srvURL+path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Key-ID", keyID)
	req.Header.Set("X-Signature", signHMAC(method, path, body, secret))
	resp, err := client.Do(req)
	require.NoError(t, err)
	return resp
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestInternal_RequiresHMAC(t *testing.T) {
	srv, _ := ingestServer(t)
	client := srv.Client()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/internal/data/sources", nil)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.NotEqual(t, http.StatusOK, resp.StatusCode, "missing HMAC headers must be rejected")
}

func TestInternal_WrongSignature_Returns401(t *testing.T) {
	srv, db := ingestServer(t)
	keyID, _ := seedHMACKey(t, db)
	client := srv.Client()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/internal/data/sources", nil)
	req.Header.Set("X-Key-ID", keyID)
	req.Header.Set("X-Signature", "hmac-sha256 deadbeef")
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestInternal_ListSources_Empty(t *testing.T) {
	srv, db := ingestServer(t)
	keyID, secret := seedHMACKey(t, db)
	client := srv.Client()
	resp := doInternal(t, client, srv.URL, http.MethodGet, "/api/v1/internal/data/sources", nil, keyID, secret)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestInternal_CreateSource_AndListJobs(t *testing.T) {
	srv, db := ingestServer(t)
	keyID, secret := seedHMACKey(t, db)
	client := srv.Client()

	body := []byte(`{"name":"users_src","source_type":"db_table","config":"{\"table\":\"users\"}"}`)
	resp := doInternal(t, client, srv.URL, http.MethodPost, "/api/v1/internal/data/sources", body, keyID, secret)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// Lookup ID
	var srcID uint64
	db.QueryRow(`SELECT id FROM ingest_sources WHERE name='users_src'`).Scan(&srcID)
	assert.NotZero(t, srcID)

	// Trigger a job
	jobBody := []byte(`{"source_id":` + itoa(srcID) + `}`)
	resp = doInternal(t, client, srv.URL, http.MethodPost, "/api/v1/internal/data/jobs", jobBody, keyID, secret)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// List jobs
	resp = doInternal(t, client, srv.URL, http.MethodGet, "/api/v1/internal/data/jobs", nil, keyID, secret)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func itoa(n uint64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
