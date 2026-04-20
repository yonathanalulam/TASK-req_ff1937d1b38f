package ingest_test

// Route-level authorization for the Data Operator surface.
//
// /api/v1/dataops/* is a session-authenticated operational console for
// ingestion + lakehouse. It must be accessible to data_operator AND
// administrator roles, and forbidden to everyone else.

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

func dataopsServer(t *testing.T) (*httptest.Server, *sql.DB) {
	t.Helper()
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"hmac_keys",
		"lakehouse_lineage", "lakehouse_metadata",
		"lakehouse_schema_versions", "lakehouse_lifecycle_policies", "legal_holds",
		"ingest_checkpoints", "ingest_jobs", "ingest_sources",
		"login_attempts", "sessions", "user_roles", "user_preferences", "users",
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

type opsJar struct{ m map[string][]*http.Cookie }

func newOpsJar() *opsJar                                  { return &opsJar{m: make(map[string][]*http.Cookie)} }
func (j *opsJar) SetCookies(u *url.URL, c []*http.Cookie) { j.m[u.Host] = append(j.m[u.Host], c...) }
func (j *opsJar) Cookies(u *url.URL) []*http.Cookie       { return j.m[u.Host] }

func seedOpsUser(t *testing.T, db *sql.DB, username, role string) {
	t.Helper()
	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		username, username+"@t.l", seedPwHash, username)
	db.Exec(`INSERT INTO user_roles (user_id, role_id)
		SELECT u.id, r.id FROM users u JOIN roles r ON u.username=? AND r.name=?`, username, role)
}

func opsLogin(t *testing.T, cli *http.Client, base, username string) string {
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

func TestHTTP_DataOpsSources_DataOperatorAllowed(t *testing.T) {
	srv, db := dataopsServer(t)
	seedOpsUser(t, db, "do_user", "data_operator")

	cli := srv.Client()
	cli.Jar = newOpsJar()
	_ = opsLogin(t, cli, srv.URL, "do_user")

	resp, err := cli.Get(srv.URL + "/api/v1/dataops/sources")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"data_operator must be allowed to list sources via the dataops console")
}

func TestHTTP_DataOpsSources_AdminAllowed(t *testing.T) {
	srv, db := dataopsServer(t)
	seedOpsUser(t, db, "do_admin", "administrator")

	cli := srv.Client()
	cli.Jar = newOpsJar()
	_ = opsLogin(t, cli, srv.URL, "do_admin")

	resp, err := cli.Get(srv.URL + "/api/v1/dataops/sources")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"administrator must keep access (superset of data_operator)")
}

func TestHTTP_DataOpsSources_RegularUserForbidden(t *testing.T) {
	srv, db := dataopsServer(t)
	seedOpsUser(t, db, "do_regular", "regular_user")

	cli := srv.Client()
	cli.Jar = newOpsJar()
	_ = opsLogin(t, cli, srv.URL, "do_regular")

	resp, err := cli.Get(srv.URL + "/api/v1/dataops/sources")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode,
		"regular users must not access the Data Operations console")
}

func TestHTTP_DataOpsSources_UnauthenticatedUnauthorized(t *testing.T) {
	srv, _ := dataopsServer(t)
	resp, err := http.Get(srv.URL + "/api/v1/dataops/sources")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"unauthenticated requests must 401 before the RBAC gate")
}
