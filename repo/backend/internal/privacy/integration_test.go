package privacy_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eagle-point/service-portal/internal/audit"
	"github.com/eagle-point/service-portal/internal/config"
	"github.com/eagle-point/service-portal/internal/privacy"
	"github.com/eagle-point/service-portal/internal/router"
	"github.com/eagle-point/service-portal/internal/testutil"
)

const seedPwHash = "$2a$10$92IXUNpkjO0rOQ5byMi.Ye4oKoEa3Ro9llC/.og/at2.uheWG/igi"

func privacyServer(t *testing.T) (*httptest.Server, *sql.DB) {
	t.Helper()
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"audit_logs",
		"data_export_requests", "data_deletion_requests",
		"notification_outbox", "notifications", "notification_templates",
		"violation_records", "moderation_actions", "moderation_queue", "sensitive_terms",
		"qa_posts", "qa_threads",
		"review_reports", "review_images", "reviews",
		"ticket_attachments", "ticket_notes", "tickets",
		"service_offerings", "service_categories",
		"addresses", "login_attempts", "sessions", "user_preferences", "user_roles", "users",
	)
	cfg := &config.Config{
		AppEnv: "test", Port: "8080", DBHost: "db", DBPort: "3306",
		FieldEncryptionKey: "", SessionCookieDomain: "localhost",
	}
	r := router.New(cfg, db)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, db
}

func doPrivJSON(t *testing.T, client *http.Client, method, url string, body any, csrf string) *http.Response {
	t.Helper()
	var buf *bytes.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		buf = bytes.NewReader(raw)
	} else {
		buf = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, url, buf)
	req.Header.Set("Content-Type", "application/json")
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	resp, err := client.Do(req)
	require.NoError(t, err)
	return resp
}

type privJar struct{ m map[string][]*http.Cookie }

func newPrivJar() *privJar                                  { return &privJar{m: make(map[string][]*http.Cookie)} }
func (j *privJar) SetCookies(u *url.URL, c []*http.Cookie)  { j.m[u.Host] = append(j.m[u.Host], c...) }
func (j *privJar) Cookies(u *url.URL) []*http.Cookie        { return j.m[u.Host] }

func seedPrivUser(t *testing.T, db *sql.DB, username, email, role string) uint64 {
	t.Helper()
	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		username, email, seedPwHash, username)
	var id uint64
	db.QueryRow(`SELECT id FROM users WHERE username=?`, username).Scan(&id)
	db.Exec(`INSERT INTO user_roles (user_id, role_id) SELECT ?, id FROM roles WHERE name=?`, id, role)
	db.Exec(`INSERT INTO user_preferences (user_id, notify_in_app) VALUES (?, 1)`, id)
	return id
}

func loginPriv(t *testing.T, client *http.Client, base, username string) string {
	t.Helper()
	resp := doPrivJSON(t, client, http.MethodPost, base+"/api/v1/auth/login",
		map[string]string{"username": username, "password": "password"}, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	return body["csrf_token"].(string)
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestHTTP_RequestExport(t *testing.T) {
	srv, db := privacyServer(t)
	seedPrivUser(t, db, "expu", "ex@t.l", "regular_user")

	client := srv.Client()
	client.Jar = newPrivJar()
	csrf := loginPriv(t, client, srv.URL, "expu")

	resp := doPrivJSON(t, client, http.MethodPost,
		srv.URL+"/api/v1/users/me/export-request", nil, csrf)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	var n int
	db.QueryRow(`SELECT COUNT(*) FROM data_export_requests WHERE status='pending'`).Scan(&n)
	assert.Equal(t, 1, n)

	// Audit log entry was written
	db.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action='data_export_requested'`).Scan(&n)
	assert.Equal(t, 1, n, "audit log entry should be recorded")
}

func TestHTTP_RequestExport_DuplicateReturns409(t *testing.T) {
	srv, db := privacyServer(t)
	seedPrivUser(t, db, "dupe", "du@t.l", "regular_user")
	client := srv.Client()
	client.Jar = newPrivJar()
	csrf := loginPriv(t, client, srv.URL, "dupe")

	resp := doPrivJSON(t, client, http.MethodPost,
		srv.URL+"/api/v1/users/me/export-request", nil, csrf)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	resp = doPrivJSON(t, client, http.MethodPost,
		srv.URL+"/api/v1/users/me/export-request", nil, csrf)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	_ = db
}

func TestHTTP_ExportStatus(t *testing.T) {
	srv, db := privacyServer(t)
	seedPrivUser(t, db, "stat", "st@t.l", "regular_user")
	client := srv.Client()
	client.Jar = newPrivJar()
	csrf := loginPriv(t, client, srv.URL, "stat")

	doPrivJSON(t, client, http.MethodPost,
		srv.URL+"/api/v1/users/me/export-request", nil, csrf).Body.Close()

	resp := doPrivJSON(t, client, http.MethodGet,
		srv.URL+"/api/v1/users/me/export-request/status", nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	req := body["export_request"].(map[string]interface{})
	assert.Equal(t, "pending", req["status"])
}

func TestHTTP_RequestDeletion_RequiresConfirmDELETE(t *testing.T) {
	srv, db := privacyServer(t)
	seedPrivUser(t, db, "delr", "dr@t.l", "regular_user")
	client := srv.Client()
	client.Jar = newPrivJar()
	csrf := loginPriv(t, client, srv.URL, "delr")

	// Wrong confirmation → 422
	resp := doPrivJSON(t, client, http.MethodPost,
		srv.URL+"/api/v1/users/me/deletion-request",
		map[string]string{"confirm": "yes"}, csrf)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

func TestHTTP_RequestDeletion_DeactivatesAccount(t *testing.T) {
	srv, db := privacyServer(t)
	userID := seedPrivUser(t, db, "delact", "da@t.l", "regular_user")
	client := srv.Client()
	client.Jar = newPrivJar()
	csrf := loginPriv(t, client, srv.URL, "delact")

	resp := doPrivJSON(t, client, http.MethodPost,
		srv.URL+"/api/v1/users/me/deletion-request",
		map[string]string{"confirm": "DELETE"}, csrf)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	var active bool
	db.QueryRow(`SELECT is_active FROM users WHERE id=?`, userID).Scan(&active)
	assert.False(t, active)
}

func TestHTTP_AdminHardDelete(t *testing.T) {
	srv, db := privacyServer(t)
	target := seedPrivUser(t, db, "victim", "v@t.l", "regular_user")
	seedPrivUser(t, db, "admin_hd", "ahd@t.l", "administrator")

	client := srv.Client()
	client.Jar = newPrivJar()
	csrf := loginPriv(t, client, srv.URL, "admin_hd")

	resp := doPrivJSON(t, client, http.MethodDelete,
		srv.URL+"/api/v1/admin/users/"+itoa(target), nil, csrf)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	var displayName string
	db.QueryRow(`SELECT display_name FROM users WHERE id=?`, target).Scan(&displayName)
	assert.Equal(t, "Deleted User", displayName)
}

func TestHTTP_AuditLog_AdminOnly(t *testing.T) {
	srv, db := privacyServer(t)
	seedPrivUser(t, db, "ru_audit", "ra@t.l", "regular_user")
	client := srv.Client()
	client.Jar = newPrivJar()
	loginPriv(t, client, srv.URL, "ru_audit")

	resp := doPrivJSON(t, client, http.MethodGet, srv.URL+"/api/v1/admin/audit-logs", nil, "")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// itoa is a tiny helper to avoid importing strconv just for one call site here.
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

// silence import lint for packages used transitively
var _ = context.Background
var _ = audit.NewService
var _ = privacy.NewService
