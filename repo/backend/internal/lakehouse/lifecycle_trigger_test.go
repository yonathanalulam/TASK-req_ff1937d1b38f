package lakehouse_test

// Admin-triggered lifecycle sweep: admins can run the archive+purge sweep on
// demand via POST /api/v1/admin/lakehouse/lifecycle/run. Non-admins get 403.

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eagle-point/service-portal/internal/config"
	"github.com/eagle-point/service-portal/internal/router"
	"github.com/eagle-point/service-portal/internal/testutil"
)

const seedPwHash = "$2a$10$92IXUNpkjO0rOQ5byMi.Ye4oKoEa3Ro9llC/.og/at2.uheWG/igi"

func adminServer(t *testing.T) (*httptest.Server, *sql.DB) {
	t.Helper()
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"lakehouse_lineage", "lakehouse_metadata",
		"lakehouse_schema_versions", "lakehouse_lifecycle_policies", "legal_holds",
		"ingest_checkpoints", "ingest_jobs", "ingest_sources",
		"login_attempts", "sessions", "user_roles", "user_preferences", "users",
	)
	cfg := &config.Config{
		AppEnv: "test", Port: "8080", DBHost: "db", DBPort: "3306",
		FieldEncryptionKey: "", SessionCookieDomain: "",
	}
	r := router.New(cfg, db)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, db
}

type jar struct{ m map[string][]*http.Cookie }

func newJar() *jar                                   { return &jar{m: make(map[string][]*http.Cookie)} }
func (j *jar) SetCookies(u *url.URL, c []*http.Cookie) { j.m[u.Host] = append(j.m[u.Host], c...) }
func (j *jar) Cookies(u *url.URL) []*http.Cookie       { return j.m[u.Host] }

func seedUser(t *testing.T, db *sql.DB, username, role string) {
	t.Helper()
	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		username, username+"@t.l", seedPwHash, username)
	db.Exec(`INSERT INTO user_roles (user_id, role_id)
		SELECT u.id, r.id FROM users u JOIN roles r ON u.username=? AND r.name=?`, username, role)
}

func loginJSON(t *testing.T, cli *http.Client, base, username string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": "password"})
	resp, err := cli.Post(base+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out struct{ CsrfToken string `json:"csrf_token"` }
	json.NewDecoder(resp.Body).Decode(&out)
	return out.CsrfToken
}

func postAuth(t *testing.T, cli *http.Client, u, csrf string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, u, nil)
	req.Header.Set("X-CSRF-Token", csrf)
	resp, err := cli.Do(req)
	require.NoError(t, err)
	return resp
}

func TestHTTP_AdminRunLifecycle_AdminOK(t *testing.T) {
	srv, db := adminServer(t)
	seedUser(t, db, "lcadmin", "administrator")

	cli := srv.Client()
	cli.Jar = newJar()
	csrf := loginJSON(t, cli, srv.URL, "lcadmin")

	resp := postAuth(t, cli, srv.URL+"/api/v1/admin/lakehouse/lifecycle/run", csrf)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Lifecycle   map[string]int `json:"lifecycle"`
		ArchiveDays int            `json:"archive_days"`
		PurgeDays   int            `json:"purge_days"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.NotNil(t, body.Lifecycle, "response must include a lifecycle summary")
	assert.Greater(t, body.ArchiveDays, 0)
	assert.Greater(t, body.PurgeDays, 0)
}

func TestHTTP_AdminRunLifecycle_RegularUserForbidden(t *testing.T) {
	srv, db := adminServer(t)
	seedUser(t, db, "lcreg", "regular_user")

	cli := srv.Client()
	cli.Jar = newJar()
	csrf := loginJSON(t, cli, srv.URL, "lcreg")

	resp := postAuth(t, cli, srv.URL+"/api/v1/admin/lakehouse/lifecycle/run", csrf)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}
