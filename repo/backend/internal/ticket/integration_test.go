package ticket_test

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

// ─── Helpers ────────────────────────────────────────────────────────────────

func ticketServerWithDB(t *testing.T) (*httptest.Server, *sql.DB) {
	t.Helper()
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"review_reports", "review_images", "reviews",
		"qa_posts", "qa_threads",
		"ticket_attachments", "ticket_notes", "tickets",
		"service_offerings", "service_categories",
		"addresses",
		"login_attempts", "sessions", "user_roles", "users",
	)
	cfg := &config.Config{
		AppEnv:              "test",
		Port:                "8080",
		DBHost:              "db",
		DBPort:              "3306",
		FieldEncryptionKey:  "",
		SessionCookieDomain: "localhost",
	}
	r := router.New(cfg, db)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, db
}

func doJSONt(t *testing.T, client *http.Client, method, url string, body any, csrf string) *http.Response {
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

type ticketCookieJar struct{ m map[string][]*http.Cookie }

func newTicketJar() *ticketCookieJar { return &ticketCookieJar{m: make(map[string][]*http.Cookie)} }
func (j *ticketCookieJar) SetCookies(u *url.URL, c []*http.Cookie) {
	j.m[u.Host] = append(j.m[u.Host], c...)
}
func (j *ticketCookieJar) Cookies(u *url.URL) []*http.Cookie { return j.m[u.Host] }

const seedPwHash = "$2a$10$92IXUNpkjO0rOQ5byMi.Ye4oKoEa3Ro9llC/.og/at2.uheWG/igi"

// seedUserWithRole inserts a user with a given role and returns its ID.
func seedUserWithRole(t *testing.T, db *sql.DB, username, email, roleName string) uint64 {
	t.Helper()
	db.Exec(
		`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		username, email, seedPwHash, username,
	)
	var id uint64
	db.QueryRow(`SELECT id FROM users WHERE username=?`, username).Scan(&id)
	if roleName != "regular_user" {
		db.Exec(`INSERT INTO user_roles (user_id, role_id) SELECT ?, id FROM roles WHERE name = ?`, id, roleName)
	} else {
		// regular_user is default role — still wire it up
		db.Exec(`INSERT INTO user_roles (user_id, role_id) SELECT ?, id FROM roles WHERE name = 'regular_user'`, id)
	}
	return id
}

// loginSeed logs in a seeded user (password "password") and returns CSRF token.
func loginSeed(t *testing.T, client *http.Client, base, username string) string {
	t.Helper()
	resp := doJSONt(t, client, http.MethodPost, base+"/api/v1/auth/login",
		map[string]string{"username": username, "password": "password"}, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	return body["csrf_token"].(string)
}

// seedTicketDependencies inserts a category, offering (owned by agentID), and address (for userID).
func seedTicketDependencies(t *testing.T, db *sql.DB, agentID, userID uint64) (catID, offID, addrID uint64) {
	t.Helper()
	db.Exec(`INSERT INTO service_categories (name, slug, response_time_minutes) VALUES (?,?,?)`,
		"ICat", "i-cat", 60)
	db.QueryRow(`SELECT id FROM service_categories WHERE slug='i-cat'`).Scan(&catID)
	db.Exec(`INSERT INTO service_offerings (agent_id, category_id, name, duration_minutes) VALUES (?,?,?,?)`,
		agentID, catID, "IOff", 60)
	db.QueryRow(`SELECT id FROM service_offerings WHERE name='IOff'`).Scan(&offID)
	db.Exec(`INSERT INTO addresses (user_id, label, address_line1_encrypted, city, state, zip, is_default)
			VALUES (?,?,?,?,?,?,1)`, userID, "Home", []byte("1 Main"), "NYC", "NY", "10001")
	db.QueryRow(`SELECT id FROM addresses WHERE user_id=?`, userID).Scan(&addrID)
	return
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestHTTP_CreateTicket_AsUser(t *testing.T) {
	srv, db := ticketServerWithDB(t)

	agentID := seedUserWithRole(t, db, "tagent", "ta@t.l", "service_agent")
	userID := seedUserWithRole(t, db, "tuser", "tu@t.l", "regular_user")
	catID, offID, addrID := seedTicketDependencies(t, db, agentID, userID)

	client := srv.Client()
	client.Jar = newTicketJar()
	csrf := loginSeed(t, client, srv.URL, "tuser")

	resp := doJSONt(t, client, http.MethodPost, srv.URL+"/api/v1/tickets",
		map[string]interface{}{
			"offering_id":     offID,
			"category_id":     catID,
			"address_id":      addrID,
			"preferred_start": "2026-06-01T10:00:00Z",
			"preferred_end":   "2026-06-01T12:00:00Z",
			"delivery_method": "pickup",
		}, csrf)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	ticket := body["ticket"].(map[string]interface{})
	assert.Equal(t, "Accepted", ticket["status"])
	assert.Equal(t, float64(userID), ticket["user_id"])
	_ = userID
}

func TestHTTP_ListTickets_RegularUserSeesOwnOnly(t *testing.T) {
	srv, db := ticketServerWithDB(t)

	agentID := seedUserWithRole(t, db, "la", "la@t.l", "service_agent")
	userA := seedUserWithRole(t, db, "lua", "lua@t.l", "regular_user")
	userB := seedUserWithRole(t, db, "lub", "lub@t.l", "regular_user")
	catID, offID, addrA := seedTicketDependencies(t, db, agentID, userA)

	// Also give userB an address
	db.Exec(`INSERT INTO addresses (user_id, label, address_line1_encrypted, city, state, zip, is_default)
			VALUES (?,?,?,?,?,?,1)`, userB, "Home", []byte("1 Main"), "X", "NY", "10001")
	var addrB uint64
	db.QueryRow(`SELECT id FROM addresses WHERE user_id=?`, userB).Scan(&addrB)

	// Two tickets directly
	db.Exec(`INSERT INTO tickets (user_id, offering_id, category_id, address_id, preferred_start, preferred_end,
			delivery_method, status) VALUES (?,?,?,?,'2026-06-01 10:00:00','2026-06-01 12:00:00','pickup','Accepted')`,
		userA, offID, catID, addrA)
	db.Exec(`INSERT INTO tickets (user_id, offering_id, category_id, address_id, preferred_start, preferred_end,
			delivery_method, status) VALUES (?,?,?,?,'2026-06-01 10:00:00','2026-06-01 12:00:00','pickup','Accepted')`,
		userB, offID, catID, addrB)

	client := srv.Client()
	client.Jar = newTicketJar()
	loginSeed(t, client, srv.URL, "lua")

	resp := doJSONt(t, client, http.MethodGet, srv.URL+"/api/v1/tickets", nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()

	tickets := body["tickets"].([]interface{})
	assert.Len(t, tickets, 1)
	owner := tickets[0].(map[string]interface{})["user_id"].(float64)
	assert.Equal(t, float64(userA), owner)
}

func TestHTTP_ListTickets_AdminSeesAll(t *testing.T) {
	srv, db := ticketServerWithDB(t)

	agentID := seedUserWithRole(t, db, "adag", "adag@t.l", "service_agent")
	userA := seedUserWithRole(t, db, "adua", "adua@t.l", "regular_user")
	userB := seedUserWithRole(t, db, "adub", "adub@t.l", "regular_user")
	seedUserWithRole(t, db, "admin", "admin@example.local", "administrator")
	catID, offID, addrA := seedTicketDependencies(t, db, agentID, userA)
	db.Exec(`INSERT INTO addresses (user_id, label, address_line1_encrypted, city, state, zip, is_default)
			VALUES (?,?,?,?,?,?,1)`, userB, "Home", []byte("1 Main"), "X", "NY", "10001")
	var addrB uint64
	db.QueryRow(`SELECT id FROM addresses WHERE user_id=?`, userB).Scan(&addrB)

	db.Exec(`INSERT INTO tickets (user_id, offering_id, category_id, address_id, preferred_start, preferred_end,
			delivery_method, status) VALUES (?,?,?,?,'2026-06-01 10:00:00','2026-06-01 12:00:00','pickup','Accepted')`,
		userA, offID, catID, addrA)
	db.Exec(`INSERT INTO tickets (user_id, offering_id, category_id, address_id, preferred_start, preferred_end,
			delivery_method, status) VALUES (?,?,?,?,'2026-06-01 10:00:00','2026-06-01 12:00:00','pickup','Accepted')`,
		userB, offID, catID, addrB)

	client := srv.Client()
	client.Jar = newTicketJar()
	loginSeed(t, client, srv.URL, "admin")

	resp := doJSONt(t, client, http.MethodGet, srv.URL+"/api/v1/tickets", nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	assert.Len(t, body["tickets"].([]interface{}), 2)
}

func TestHTTP_UpdateStatus_AgentLifecycle(t *testing.T) {
	srv, db := ticketServerWithDB(t)

	agentID := seedUserWithRole(t, db, "agentlife", "al@t.l", "service_agent")
	userID := seedUserWithRole(t, db, "userlife", "ul@t.l", "regular_user")
	catID, offID, addrID := seedTicketDependencies(t, db, agentID, userID)

	// Seed a ticket in Accepted state
	db.Exec(`INSERT INTO tickets (user_id, offering_id, category_id, address_id, preferred_start, preferred_end,
			delivery_method, status) VALUES (?,?,?,?,'2026-06-01 10:00:00','2026-06-01 12:00:00','pickup','Accepted')`,
		userID, offID, catID, addrID)
	var ticketID uint64
	db.QueryRow(`SELECT id FROM tickets WHERE user_id=?`, userID).Scan(&ticketID)

	client := srv.Client()
	client.Jar = newTicketJar()
	csrf := loginSeed(t, client, srv.URL, "agentlife")

	url := srv.URL + "/api/v1/tickets/" + strconv.FormatUint(ticketID, 10) + "/status"
	for _, step := range []string{"Dispatched", "In Service", "Completed"} {
		resp := doJSONt(t, client, http.MethodPatch, url,
			map[string]string{"status": step}, csrf)
		require.Equal(t, http.StatusOK, resp.StatusCode, "agent should move to %s", step)
		resp.Body.Close()
	}
}

func TestHTTP_UpdateStatus_InvalidTransitionFor422(t *testing.T) {
	srv, db := ticketServerWithDB(t)

	agentID := seedUserWithRole(t, db, "agentbad", "ab@t.l", "service_agent")
	userID := seedUserWithRole(t, db, "userbad", "ub@t.l", "regular_user")
	catID, offID, addrID := seedTicketDependencies(t, db, agentID, userID)
	db.Exec(`INSERT INTO tickets (user_id, offering_id, category_id, address_id, preferred_start, preferred_end,
			delivery_method, status) VALUES (?,?,?,?,'2026-06-01 10:00:00','2026-06-01 12:00:00','pickup','Accepted')`,
		userID, offID, catID, addrID)
	var ticketID uint64
	db.QueryRow(`SELECT id FROM tickets WHERE user_id=?`, userID).Scan(&ticketID)

	client := srv.Client()
	client.Jar = newTicketJar()
	csrf := loginSeed(t, client, srv.URL, "agentbad")

	// Agent can't skip to Completed from Accepted
	url := srv.URL + "/api/v1/tickets/" + strconv.FormatUint(ticketID, 10) + "/status"
	resp := doJSONt(t, client, http.MethodPatch, url,
		map[string]string{"status": "Completed"}, csrf)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

func TestHTTP_UpdateStatus_UserCancelsBeforeDispatch(t *testing.T) {
	srv, db := ticketServerWithDB(t)

	agentID := seedUserWithRole(t, db, "cag", "cag@t.l", "service_agent")
	userID := seedUserWithRole(t, db, "cuser", "cu@t.l", "regular_user")
	catID, offID, addrID := seedTicketDependencies(t, db, agentID, userID)
	db.Exec(`INSERT INTO tickets (user_id, offering_id, category_id, address_id, preferred_start, preferred_end,
			delivery_method, status) VALUES (?,?,?,?,'2026-06-01 10:00:00','2026-06-01 12:00:00','pickup','Accepted')`,
		userID, offID, catID, addrID)
	var ticketID uint64
	db.QueryRow(`SELECT id FROM tickets WHERE user_id=?`, userID).Scan(&ticketID)

	client := srv.Client()
	client.Jar = newTicketJar()
	csrf := loginSeed(t, client, srv.URL, "cuser")

	url := srv.URL + "/api/v1/tickets/" + strconv.FormatUint(ticketID, 10) + "/status"
	resp := doJSONt(t, client, http.MethodPatch, url,
		map[string]interface{}{"status": "Cancelled", "cancel_reason": "changed my mind"}, csrf)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestHTTP_UpdateStatus_UserCancelAfterDispatch_Rejected(t *testing.T) {
	srv, db := ticketServerWithDB(t)

	agentID := seedUserWithRole(t, db, "lac", "lac@t.l", "service_agent")
	userID := seedUserWithRole(t, db, "lacu", "lacu@t.l", "regular_user")
	catID, offID, addrID := seedTicketDependencies(t, db, agentID, userID)
	db.Exec(`INSERT INTO tickets (user_id, offering_id, category_id, address_id, preferred_start, preferred_end,
			delivery_method, status) VALUES (?,?,?,?,'2026-06-01 10:00:00','2026-06-01 12:00:00','pickup','Dispatched')`,
		userID, offID, catID, addrID)
	var ticketID uint64
	db.QueryRow(`SELECT id FROM tickets WHERE user_id=?`, userID).Scan(&ticketID)

	client := srv.Client()
	client.Jar = newTicketJar()
	csrf := loginSeed(t, client, srv.URL, "lacu")

	url := srv.URL + "/api/v1/tickets/" + strconv.FormatUint(ticketID, 10) + "/status"
	resp := doJSONt(t, client, http.MethodPatch, url,
		map[string]string{"status": "Cancelled"}, csrf)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

func TestHTTP_Notes_CreateAndList(t *testing.T) {
	srv, db := ticketServerWithDB(t)

	agentID := seedUserWithRole(t, db, "na", "na@t.l", "service_agent")
	userID := seedUserWithRole(t, db, "nu", "nu@t.l", "regular_user")
	catID, offID, addrID := seedTicketDependencies(t, db, agentID, userID)
	db.Exec(`INSERT INTO tickets (user_id, offering_id, category_id, address_id, preferred_start, preferred_end,
			delivery_method, status) VALUES (?,?,?,?,'2026-06-01 10:00:00','2026-06-01 12:00:00','pickup','Accepted')`,
		userID, offID, catID, addrID)
	var ticketID uint64
	db.QueryRow(`SELECT id FROM tickets WHERE user_id=?`, userID).Scan(&ticketID)

	client := srv.Client()
	client.Jar = newTicketJar()
	csrf := loginSeed(t, client, srv.URL, "nu")

	idStr := strconv.FormatUint(ticketID, 10)
	resp := doJSONt(t, client, http.MethodPost, srv.URL+"/api/v1/tickets/"+idStr+"/notes",
		map[string]string{"content": "Please arrive through side gate"}, csrf)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	resp = doJSONt(t, client, http.MethodGet, srv.URL+"/api/v1/tickets/"+idStr+"/notes", nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()

	notes := body["notes"].([]interface{})
	assert.Len(t, notes, 1)
}

func TestHTTP_RequiresAuth(t *testing.T) {
	srv, _ := ticketServerWithDB(t)
	client := srv.Client()

	resp := doJSONt(t, client, http.MethodGet, srv.URL+"/api/v1/tickets", nil, "")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}
