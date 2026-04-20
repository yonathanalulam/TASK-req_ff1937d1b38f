package catalog_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eagle-point/service-portal/internal/config"
	"github.com/eagle-point/service-portal/internal/router"
	"github.com/eagle-point/service-portal/internal/testutil"
)

// ─── HTTP test helpers ────────────────────────────────────────────────────────

// catalogServerWithDB starts the test server and returns both the server and the
// underlying DB so tests can insert seed data directly.
func catalogServerWithDB(t *testing.T) (*httptest.Server, *sql.DB) {
	t.Helper()
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"user_browsing_history", "user_favorites",
		"service_offerings", "service_categories",
		"shipping_templates", "shipping_regions",
		"login_attempts", "sessions", "user_roles", "users",
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
	return srv, db
}

func doCatalogJSON(t *testing.T, client *http.Client, method, url string, body any, csrfToken string) *http.Response {
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
	if csrfToken != "" {
		req.Header.Set("X-CSRF-Token", csrfToken)
	}
	resp, err := client.Do(req)
	require.NoError(t, err)
	return resp
}

// registerAndLogin creates a user account and returns the CSRF token.
func registerAndLogin(t *testing.T, client *http.Client, base, username string) string {
	t.Helper()
	r := doCatalogJSON(t, client, http.MethodPost, base+"/api/v1/auth/register", map[string]string{
		"username": username, "email": username + "@test.local",
		"password": "ValidPass1", "display_name": username,
	}, "")
	r.Body.Close()
	require.Equal(t, http.StatusCreated, r.StatusCode)

	resp := doCatalogJSON(t, client, http.MethodPost, base+"/api/v1/auth/login", map[string]string{
		"username": username, "password": "ValidPass1",
	}, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	return body["csrf_token"].(string)
}

type catalogCookieJar struct {
	cookies map[string][]*http.Cookie
}

func newCatalogCookieJar() *catalogCookieJar {
	return &catalogCookieJar{cookies: make(map[string][]*http.Cookie)}
}
func (j *catalogCookieJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.cookies[u.Host] = append(j.cookies[u.Host], cookies...)
}
func (j *catalogCookieJar) Cookies(u *url.URL) []*http.Cookie {
	return j.cookies[u.Host]
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestHTTP_ListCategories_PublicAccess(t *testing.T) {
	srv, _ := catalogServerWithDB(t)
	client := srv.Client()

	resp := doCatalogJSON(t, client, http.MethodGet, srv.URL+"/api/v1/service-categories", nil, "")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	cats := body["categories"].([]interface{})
	assert.NotNil(t, cats) // may be empty, but key must exist
}

func TestHTTP_ListOfferings_RequiresAuth(t *testing.T) {
	srv, _ := catalogServerWithDB(t)
	client := srv.Client()

	resp := doCatalogJSON(t, client, http.MethodGet, srv.URL+"/api/v1/service-offerings", nil, "")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestHTTP_CreateOffering_RequiresServiceAgentRole(t *testing.T) {
	srv, _ := catalogServerWithDB(t)

	// Regular user (default role after register)
	client := srv.Client()
	client.Jar = newCatalogCookieJar()
	csrf := registerAndLogin(t, client, srv.URL, "regularcat")

	resp := doCatalogJSON(t, client, http.MethodPost, srv.URL+"/api/v1/service-offerings",
		map[string]interface{}{
			"category_id": 1, "name": "My Service",
			"base_price": 10.0, "duration_minutes": 60,
		}, csrf)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestHTTP_ListOfferings_FilterByCategory(t *testing.T) {
	srv, db := catalogServerWithDB(t)

	// Insert a service_agent user directly (with bcrypt hash for "password")
	db.Exec(
		`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"httpagent", "httpagent@test.local",
		"$2a$10$92IXUNpkjO0rOQ5byMi.Ye4oKoEa3Ro9llC/.og/at2.uheWG/igi",
		"HTTP Agent",
	)
	var agentID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='httpagent'`).Scan(&agentID)
	db.Exec(`INSERT INTO user_roles (user_id, role_id) SELECT ?, id FROM roles WHERE name = 'service_agent'`, agentID)

	db.Exec(
		`INSERT INTO service_categories (name, slug) VALUES (?,?), (?,?)`,
		"HTTP Cat A", "http-cat-a", "HTTP Cat B", "http-cat-b",
	)
	var catAID, catBID uint64
	db.QueryRow(`SELECT id FROM service_categories WHERE slug='http-cat-a'`).Scan(&catAID)
	db.QueryRow(`SELECT id FROM service_categories WHERE slug='http-cat-b'`).Scan(&catBID)

	db.Exec(
		`INSERT INTO service_offerings (agent_id, category_id, name, duration_minutes) VALUES (?,?,?,?),(?,?,?,?),(?,?,?,?)`,
		agentID, catAID, "A1", 30,
		agentID, catAID, "A2", 30,
		agentID, catBID, "B1", 30,
	)

	// Log in as the service agent
	client := srv.Client()
	client.Jar = newCatalogCookieJar()
	resp := doCatalogJSON(t, client, http.MethodPost, srv.URL+"/api/v1/auth/login", map[string]string{
		"username": "httpagent", "password": "password",
	}, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Filter by catA
	url := srv.URL + "/api/v1/service-offerings?category_id=" + strconv.FormatUint(catAID, 10)
	resp = doCatalogJSON(t, client, http.MethodGet, url, nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()

	items := body["items"].([]interface{})
	assert.Len(t, items, 2)
	for _, item := range items {
		o := item.(map[string]interface{})
		assert.Equal(t, float64(catAID), o["category_id"])
	}
}

func TestHTTP_UpdateOffering_NonOwner_Returns403(t *testing.T) {
	srv, db := catalogServerWithDB(t)

	// Insert owner and non-owner, both as service_agent (bcrypt for "password")
	const hash = "$2a$10$92IXUNpkjO0rOQ5byMi.Ye4oKoEa3Ro9llC/.og/at2.uheWG/igi"
	for _, u := range []struct{ user, email string }{
		{"httpowner", "httpowner@test.local"},
		{"httpother", "httpother@test.local"},
	} {
		db.Exec(
			`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
			u.user, u.email, hash, u.user,
		)
	}
	var ownerID, otherID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='httpowner'`).Scan(&ownerID)
	db.QueryRow(`SELECT id FROM users WHERE username='httpother'`).Scan(&otherID)
	db.Exec(`INSERT INTO user_roles (user_id, role_id) SELECT ?, id FROM roles WHERE name = 'service_agent'`, ownerID)
	db.Exec(`INSERT INTO user_roles (user_id, role_id) SELECT ?, id FROM roles WHERE name = 'service_agent'`, otherID)

	db.Exec(`INSERT INTO service_categories (name, slug) VALUES (?,?)`, "Misc", "misc-403")
	var catID uint64
	db.QueryRow(`SELECT id FROM service_categories WHERE slug='misc-403'`).Scan(&catID)
	db.Exec(
		`INSERT INTO service_offerings (agent_id, category_id, name, duration_minutes) VALUES (?,?,?,?)`,
		ownerID, catID, "Owner Offering", 60,
	)
	var offeringID uint64
	db.QueryRow(`SELECT id FROM service_offerings WHERE name='Owner Offering'`).Scan(&offeringID)

	// Log in as non-owner
	client := srv.Client()
	client.Jar = newCatalogCookieJar()
	resp := doCatalogJSON(t, client, http.MethodPost, srv.URL+"/api/v1/auth/login", map[string]string{
		"username": "httpother", "password": "password",
	}, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var loginBody map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&loginBody)
	resp.Body.Close()
	csrf := loginBody["csrf_token"].(string)

	url := srv.URL + "/api/v1/service-offerings/" + strconv.FormatUint(offeringID, 10)
	resp = doCatalogJSON(t, client, http.MethodPut, url,
		map[string]interface{}{
			"category_id": catID, "name": "Hijacked",
			"base_price": 0.0, "duration_minutes": 60,
		}, csrf)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestHTTP_GetOffering_NotFound(t *testing.T) {
	srv, _ := catalogServerWithDB(t)

	client := srv.Client()
	client.Jar = newCatalogCookieJar()
	registerAndLogin(t, client, srv.URL, "notfounduser")

	resp := doCatalogJSON(t, client, http.MethodGet, srv.URL+"/api/v1/service-offerings/999999", nil, "")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestHTTP_ListRegions_PublicAccess(t *testing.T) {
	srv, _ := catalogServerWithDB(t)
	client := srv.Client()

	resp := doCatalogJSON(t, client, http.MethodGet, srv.URL+"/api/v1/shipping/regions", nil, "")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	regions := body["regions"].([]interface{})
	assert.NotNil(t, regions)
}

func TestHTTP_ShippingEstimate_Pickup_NoWindow(t *testing.T) {
	srv, db := catalogServerWithDB(t)

	db.Exec(`INSERT INTO shipping_regions (name, cutoff_time, timezone) VALUES (?,?,?)`,
		"Test Region", "17:00:00", "UTC")
	var regionID uint64
	db.QueryRow(`SELECT id FROM shipping_regions WHERE name='Test Region'`).Scan(&regionID)

	client := srv.Client()
	client.Jar = newCatalogCookieJar()
	csrf := registerAndLogin(t, client, srv.URL, "estimateuser")

	resp := doCatalogJSON(t, client, http.MethodPost, srv.URL+"/api/v1/shipping/estimate",
		map[string]interface{}{
			"region_id":       regionID,
			"delivery_method": "pickup",
			"weight_kg":       1.0,
			"quantity":        1,
		}, csrf)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()

	assert.Equal(t, 0.0, body["fee"])
	assert.Equal(t, "USD", body["currency"])
	_, hasWindow := body["estimated_arrival_window"]
	assert.False(t, hasWindow)
}

// Stale tests previously referenced /api/v1/catalog/services and
// /api/v1/catalog/categories, neither of which are registered in
// router.go. The real routes are /api/v1/service-offerings and
// /api/v1/service-categories, both already covered above.
