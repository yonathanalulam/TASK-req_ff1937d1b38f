package profile_test

import (
	"bytes"
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

// profileServer spins up a full httptest.Server backed by a real database.
func profileServer(t *testing.T) *httptest.Server {
	t.Helper()
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"user_browsing_history", "user_favorites",
		"addresses", "user_preferences",
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
	return srv
}

// doProfileJSON is a JSON request helper with cookie/CSRF support.
func doProfileJSON(t *testing.T, client *http.Client, method, url string, body any, csrfToken string) *http.Response {
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

// loginAndGetCSRF registers and logs in a user, returning the CSRF token.
func loginAndGetCSRF(t *testing.T, client *http.Client, base, username string) string {
	t.Helper()
	// Register
	r := doProfileJSON(t, client, http.MethodPost, base+"/api/v1/auth/register", map[string]string{
		"username": username, "email": username + "@test.local",
		"password": "ValidPass1", "display_name": username,
	}, "")
	r.Body.Close()
	require.Equal(t, http.StatusCreated, r.StatusCode)

	// Login
	resp := doProfileJSON(t, client, http.MethodPost, base+"/api/v1/auth/login", map[string]string{
		"username": username, "password": "ValidPass1",
	}, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	return body["csrf_token"].(string)
}

// newProfileCookieJar returns a simple cookie jar for test clients.
func newProfileCookieJar() *profileCookieJar {
	return &profileCookieJar{cookies: make(map[string][]*http.Cookie)}
}

type profileCookieJar struct {
	cookies map[string][]*http.Cookie
}

func (j *profileCookieJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.cookies[u.Host] = append(j.cookies[u.Host], cookies...)
}
func (j *profileCookieJar) Cookies(u *url.URL) []*http.Cookie {
	return j.cookies[u.Host]
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestHTTP_GetProfile_RequiresAuth(t *testing.T) {
	srv := profileServer(t)
	client := srv.Client()

	resp := doProfileJSON(t, client, http.MethodGet, srv.URL+"/api/v1/users/me/profile", nil, "")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestHTTP_UpdateProfile_MasksPhone(t *testing.T) {
	srv := profileServer(t)
	client := srv.Client()
	client.Jar = newProfileCookieJar()

	csrf := loginAndGetCSRF(t, client, srv.URL, "maskphoneuser")

	// Update profile with phone
	resp := doProfileJSON(t, client, http.MethodPut, srv.URL+"/api/v1/users/me/profile",
		map[string]string{
			"display_name": "Mask Phone User",
			"phone":        "4155551234",
		}, csrf)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()

	p := body["profile"].(map[string]interface{})
	assert.Equal(t, "(415) ***-1234", p["phone"])
	assert.Equal(t, "Mask Phone User", p["display_name"])
}

func TestHTTP_UpdateProfile_EmptyDisplayName_Returns400(t *testing.T) {
	srv := profileServer(t)
	client := srv.Client()
	client.Jar = newProfileCookieJar()

	csrf := loginAndGetCSRF(t, client, srv.URL, "nodname")

	resp := doProfileJSON(t, client, http.MethodPut, srv.URL+"/api/v1/users/me/profile",
		map[string]string{"display_name": ""}, csrf)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestHTTP_Preferences_GetAndUpdate(t *testing.T) {
	srv := profileServer(t)
	client := srv.Client()
	client.Jar = newProfileCookieJar()

	csrf := loginAndGetCSRF(t, client, srv.URL, "prefhttpuser")

	// GET initial preferences
	resp := doProfileJSON(t, client, http.MethodGet, srv.URL+"/api/v1/users/me/preferences", nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// PUT updated preferences
	resp = doProfileJSON(t, client, http.MethodPut, srv.URL+"/api/v1/users/me/preferences",
		map[string]interface{}{
			"notify_in_app":  false,
			"muted_tags":     []int{1, 2},
			"muted_authors":  []int{},
		}, csrf)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()

	prefs := body["preferences"].(map[string]interface{})
	assert.Equal(t, false, prefs["notify_in_app"])
}

func TestHTTP_Addresses_InvalidZip_Returns422(t *testing.T) {
	srv := profileServer(t)
	client := srv.Client()
	client.Jar = newProfileCookieJar()

	csrf := loginAndGetCSRF(t, client, srv.URL, "badzip")

	resp := doProfileJSON(t, client, http.MethodPost, srv.URL+"/api/v1/users/me/addresses",
		map[string]string{
			"label": "Home", "address_line1": "1 Main St",
			"city": "NYC", "state": "NY", "zip": "BADZIP",
		}, csrf)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

func TestHTTP_Addresses_CreateListDeleteDefault(t *testing.T) {
	srv := profileServer(t)
	client := srv.Client()
	client.Jar = newProfileCookieJar()

	csrf := loginAndGetCSRF(t, client, srv.URL, "addrhttp")

	// Create first address
	resp := doProfileJSON(t, client, http.MethodPost, srv.URL+"/api/v1/users/me/addresses",
		map[string]string{
			"label": "Home", "address_line1": "100 Oak St",
			"city": "Portland", "state": "OR", "zip": "97201",
		}, csrf)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var b1 map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&b1)
	resp.Body.Close()
	addr1 := b1["address"].(map[string]interface{})
	addr1ID := addr1["id"].(float64)
	assert.Equal(t, true, addr1["is_default"])

	// Create second address
	resp = doProfileJSON(t, client, http.MethodPost, srv.URL+"/api/v1/users/me/addresses",
		map[string]string{
			"label": "Work", "address_line1": "200 Pine Ave",
			"city": "Portland", "state": "OR", "zip": "97202",
		}, csrf)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var b2 map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&b2)
	resp.Body.Close()
	addr2 := b2["address"].(map[string]interface{})
	addr2ID := uint64(addr2["id"].(float64))

	// List — should have 2 addresses
	resp = doProfileJSON(t, client, http.MethodGet, srv.URL+"/api/v1/users/me/addresses", nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var listBody map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&listBody)
	resp.Body.Close()
	addrs := listBody["addresses"].([]interface{})
	assert.Len(t, addrs, 2)

	// Set second address as default
	setDefaultURL := srv.URL + "/api/v1/users/me/addresses/" + uintToStr(addr2ID) + "/default"
	resp = doProfileJSON(t, client, http.MethodPut, setDefaultURL, nil, csrf)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var defBody map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&defBody)
	resp.Body.Close()
	defAddr := defBody["address"].(map[string]interface{})
	assert.Equal(t, true, defAddr["is_default"])

	// Delete first address
	resp = doProfileJSON(t, client, http.MethodDelete,
		srv.URL+"/api/v1/users/me/addresses/"+uintToStr(uint64(addr1ID)), nil, csrf)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	// List — should have 1 address
	resp = doProfileJSON(t, client, http.MethodGet, srv.URL+"/api/v1/users/me/addresses", nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	json.NewDecoder(resp.Body).Decode(&listBody)
	resp.Body.Close()
	assert.Len(t, listBody["addresses"].([]interface{}), 1)
}

func uintToStr(id uint64) string {
	return strconv.FormatUint(id, 10)
}

func TestProfileGet_Success(t *testing.T) {
	srv := profileServer(t)
	client := srv.Client()
	client.Jar = newProfileCookieJar()
	csrf := loginAndGetCSRF(t, client, srv.URL, "addrhttp")
	_ = csrf

	// The real router exposes the profile document at /users/me/profile;
	// the legacy /api/v1/profile path never existed.
	resp := doProfileJSON(t, client, http.MethodGet, srv.URL+"/api/v1/users/me/profile", nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	resp.Body.Close()

	profile, ok := body["profile"].(map[string]interface{})
	require.True(t, ok, "response must wrap the profile document under a profile key")
	assert.Contains(t, profile, "id")
	assert.Contains(t, profile, "username")
	assert.Contains(t, profile, "email")
	assert.Contains(t, profile, "display_name")
	assert.Equal(t, "addrhttp", profile["username"])
}

func TestProfileGet_Unauthorized(t *testing.T) {
	srv := profileServer(t)
	client := srv.Client()

	resp := doProfileJSON(t, client, http.MethodGet, srv.URL+"/api/v1/users/me/profile", nil, "")
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()
}
