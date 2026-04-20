package moderation_test

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

const seedPwHash = "$2a$10$92IXUNpkjO0rOQ5byMi.Ye4oKoEa3Ro9llC/.og/at2.uheWG/igi"

func modServer(t *testing.T) (*httptest.Server, *sql.DB) {
	t.Helper()
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
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

func doModJSON(t *testing.T, client *http.Client, method, url string, body any, csrf string) *http.Response {
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

type modJar struct{ m map[string][]*http.Cookie }

func newModJar() *modJar                                    { return &modJar{m: make(map[string][]*http.Cookie)} }
func (j *modJar) SetCookies(u *url.URL, c []*http.Cookie)   { j.m[u.Host] = append(j.m[u.Host], c...) }
func (j *modJar) Cookies(u *url.URL) []*http.Cookie         { return j.m[u.Host] }

func seedRoleUser(t *testing.T, db *sql.DB, username, email, role string) uint64 {
	t.Helper()
	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		username, email, seedPwHash, username)
	var id uint64
	db.QueryRow(`SELECT id FROM users WHERE username=?`, username).Scan(&id)
	db.Exec(`INSERT INTO user_roles (user_id, role_id) SELECT ?, id FROM roles WHERE name=?`, id, role)
	db.Exec(`INSERT INTO user_preferences (user_id, notify_in_app) VALUES (?, 1)`, id)
	return id
}

func loginMod(t *testing.T, client *http.Client, base, username string) string {
	t.Helper()
	resp := doModJSON(t, client, http.MethodPost, base+"/api/v1/auth/login",
		map[string]string{"username": username, "password": "password"}, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	return body["csrf_token"].(string)
}

// seedReviewableTicket creates an offering + completed ticket so reviews can be posted.
func seedReviewableTicket(t *testing.T, db *sql.DB, customerID uint64) (offeringID uint64, ticketID uint64) {
	t.Helper()
	// Use the customer as agent for simplicity (offering needs an agent FK)
	db.Exec(`INSERT INTO service_categories (name, slug, response_time_minutes) VALUES (?,?,?)`,
		"ModC", "modc", 60)
	var catID uint64
	db.QueryRow(`SELECT id FROM service_categories WHERE slug='modc'`).Scan(&catID)
	db.Exec(`INSERT INTO service_offerings (agent_id, category_id, name, duration_minutes) VALUES (?,?,?,?)`,
		customerID, catID, "ModO", 60)
	db.QueryRow(`SELECT id FROM service_offerings WHERE name='ModO'`).Scan(&offeringID)
	db.Exec(`INSERT INTO addresses (user_id, label, address_line1_encrypted, city, state, zip, is_default) VALUES (?,?,?,?,?,?,1)`,
		customerID, "H", []byte("1 Main"), "X", "NY", "10001")
	var addrID uint64
	db.QueryRow(`SELECT id FROM addresses WHERE user_id=?`, customerID).Scan(&addrID)
	db.Exec(`INSERT INTO tickets (user_id, offering_id, category_id, address_id, preferred_start, preferred_end,
		delivery_method, status) VALUES (?,?,?,?,'2026-06-01 10:00:00','2026-06-01 12:00:00','pickup','Completed')`,
		customerID, offeringID, catID, addrID)
	db.QueryRow(`SELECT id FROM tickets WHERE user_id=?`, customerID).Scan(&ticketID)
	return
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestHTTP_AdminAddTerm(t *testing.T) {
	srv, db := modServer(t)
	seedRoleUser(t, db, "admt", "ad@t.l", "administrator")

	client := srv.Client()
	client.Jar = newModJar()
	csrf := loginMod(t, client, srv.URL, "admt")

	resp := doModJSON(t, client, http.MethodPost, srv.URL+"/api/v1/admin/sensitive-terms",
		map[string]string{"term": "evilword", "class": "prohibited"}, csrf)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	var n int
	db.QueryRow(`SELECT COUNT(*) FROM sensitive_terms WHERE term='evilword'`).Scan(&n)
	assert.Equal(t, 1, n)
}

func TestHTTP_AdminAddTerm_NonAdmin_403(t *testing.T) {
	srv, db := modServer(t)
	seedRoleUser(t, db, "regular_x", "rx@t.l", "regular_user")

	client := srv.Client()
	client.Jar = newModJar()
	csrf := loginMod(t, client, srv.URL, "regular_x")

	resp := doModJSON(t, client, http.MethodPost, srv.URL+"/api/v1/admin/sensitive-terms",
		map[string]string{"term": "x", "class": "prohibited"}, csrf)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestHTTP_ProhibitedReview_Blocked422(t *testing.T) {
	srv, db := modServer(t)
	adminID := seedRoleUser(t, db, "ad_pro", "adpro@t.l", "administrator")
	customerID := seedRoleUser(t, db, "cust_pro", "cp@t.l", "regular_user")
	_ = adminID

	// Pre-load a prohibited term
	db.Exec(`INSERT INTO sensitive_terms (term, class) VALUES (?, 'prohibited')`, "forbidden")

	_, ticketID := seedReviewableTicket(t, db, customerID)

	client := srv.Client()
	client.Jar = newModJar()
	csrf := loginMod(t, client, srv.URL, "cust_pro")

	// First request after server start triggers the screen middleware which
	// loads the term cache lazily — but to ensure the freshly inserted term
	// is visible, hit the admin terms list first (admin login required, so
	// instead use a separate admin flow). For this test, the server's startup
	// preload may have run before the term insert; the screening cache reload
	// is normally tied to AddTerm. As a safety net, bounce the cache by
	// deleting + re-adding via the admin endpoint:
	adminClient := srv.Client()
	adminClient.Jar = newModJar()
	seedRoleUser(t, db, "preloader", "pl@t.l", "administrator")
	pcsrf := loginMod(t, adminClient, srv.URL, "preloader")
	doModJSON(t, adminClient, http.MethodPost, srv.URL+"/api/v1/admin/sensitive-terms",
		map[string]string{"term": "forbidden2", "class": "prohibited"}, pcsrf).Body.Close()

	resp := doModJSON(t, client, http.MethodPost,
		srv.URL+"/api/v1/tickets/"+strconv.FormatUint(ticketID, 10)+"/reviews",
		map[string]interface{}{"rating": 4, "text": "this contains forbidden text"}, csrf)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	errObj := body["error"].(map[string]interface{})
	assert.Equal(t, "content_blocked", errObj["code"])
}

func TestHTTP_BorderlineReview_EnqueuedForModeration(t *testing.T) {
	srv, db := modServer(t)
	adminID := seedRoleUser(t, db, "ad_bord", "adbord@t.l", "administrator")
	customerID := seedRoleUser(t, db, "cust_bord", "cb@t.l", "regular_user")
	_ = adminID

	// Add a borderline term via admin endpoint to ensure cache is fresh
	adminClient := srv.Client()
	adminClient.Jar = newModJar()
	acsrf := loginMod(t, adminClient, srv.URL, "ad_bord")
	doModJSON(t, adminClient, http.MethodPost, srv.URL+"/api/v1/admin/sensitive-terms",
		map[string]string{"term": "warnword", "class": "borderline"}, acsrf).Body.Close()

	_, ticketID := seedReviewableTicket(t, db, customerID)

	client := srv.Client()
	client.Jar = newModJar()
	csrf := loginMod(t, client, srv.URL, "cust_bord")

	resp := doModJSON(t, client, http.MethodPost,
		srv.URL+"/api/v1/tickets/"+strconv.FormatUint(ticketID, 10)+"/reviews",
		map[string]interface{}{"rating": 3, "text": "this has a warnword in it"}, csrf)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// The review should be in moderation queue
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM moderation_queue WHERE content_type='review' AND status='pending'`).Scan(&n)
	assert.Equal(t, 1, n)

	// And the review row should be flipped to pending_moderation
	var status string
	db.QueryRow(`SELECT status FROM reviews WHERE ticket_id=?`, ticketID).Scan(&status)
	assert.Equal(t, "pending_moderation", status)
}

func TestHTTP_ModeratorApproveQueueItem(t *testing.T) {
	srv, db := modServer(t)
	seedRoleUser(t, db, "ad_aprv", "adaprv@t.l", "administrator")
	modID := seedRoleUser(t, db, "mod_aprv", "modaprv@t.l", "moderator")
	customerID := seedRoleUser(t, db, "cust_aprv", "ca@t.l", "regular_user")
	_ = modID

	adminClient := srv.Client()
	adminClient.Jar = newModJar()
	acsrf := loginMod(t, adminClient, srv.URL, "ad_aprv")
	doModJSON(t, adminClient, http.MethodPost, srv.URL+"/api/v1/admin/sensitive-terms",
		map[string]string{"term": "softword", "class": "borderline"}, acsrf).Body.Close()

	_, ticketID := seedReviewableTicket(t, db, customerID)
	custClient := srv.Client()
	custClient.Jar = newModJar()
	ccsrf := loginMod(t, custClient, srv.URL, "cust_aprv")
	doModJSON(t, custClient, http.MethodPost,
		srv.URL+"/api/v1/tickets/"+strconv.FormatUint(ticketID, 10)+"/reviews",
		map[string]interface{}{"rating": 4, "text": "softword review here"}, ccsrf).Body.Close()

	// Get queue id
	var queueID uint64
	db.QueryRow(`SELECT id FROM moderation_queue WHERE status='pending'`).Scan(&queueID)
	require.NotZero(t, queueID)

	// Moderator approves
	modClient := srv.Client()
	modClient.Jar = newModJar()
	mcsrf := loginMod(t, modClient, srv.URL, "mod_aprv")
	resp := doModJSON(t, modClient, http.MethodPost,
		srv.URL+"/api/v1/moderation/queue/"+strconv.FormatUint(queueID, 10)+"/approve",
		map[string]string{"reason": "looks fine"}, mcsrf)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Verify review status promoted back to published
	var status string
	db.QueryRow(`SELECT status FROM reviews WHERE ticket_id=?`, ticketID).Scan(&status)
	assert.Equal(t, "published", status)

	// Action recorded
	var actions int
	db.QueryRow(`SELECT COUNT(*) FROM moderation_actions WHERE action_type='approve'`).Scan(&actions)
	assert.Equal(t, 1, actions)
}

func TestHTTP_ModeratorRejectQueueItem_AppliesFreeze(t *testing.T) {
	srv, db := modServer(t)
	seedRoleUser(t, db, "ad_rj", "adrj@t.l", "administrator")
	seedRoleUser(t, db, "mod_rj", "modrj@t.l", "moderator")
	customerID := seedRoleUser(t, db, "cust_rj", "cr@t.l", "regular_user")

	adminClient := srv.Client()
	adminClient.Jar = newModJar()
	acsrf := loginMod(t, adminClient, srv.URL, "ad_rj")
	doModJSON(t, adminClient, http.MethodPost, srv.URL+"/api/v1/admin/sensitive-terms",
		map[string]string{"term": "rudeword", "class": "borderline"}, acsrf).Body.Close()

	_, ticketID := seedReviewableTicket(t, db, customerID)

	custClient := srv.Client()
	custClient.Jar = newModJar()
	ccsrf := loginMod(t, custClient, srv.URL, "cust_rj")
	doModJSON(t, custClient, http.MethodPost,
		srv.URL+"/api/v1/tickets/"+strconv.FormatUint(ticketID, 10)+"/reviews",
		map[string]interface{}{"rating": 1, "text": "rudeword content"}, ccsrf).Body.Close()

	var queueID uint64
	db.QueryRow(`SELECT id FROM moderation_queue WHERE status='pending'`).Scan(&queueID)

	modClient := srv.Client()
	modClient.Jar = newModJar()
	mcsrf := loginMod(t, modClient, srv.URL, "mod_rj")
	resp := doModJSON(t, modClient, http.MethodPost,
		srv.URL+"/api/v1/moderation/queue/"+strconv.FormatUint(queueID, 10)+"/reject",
		map[string]string{"reason": "abusive"}, mcsrf)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	assert.Contains(t, body, "freeze_until", "reject should report freeze_until on first violation")

	// Customer's posting_freeze_until should be set
	var until sql.NullTime
	db.QueryRow(`SELECT posting_freeze_until FROM users WHERE id=?`, customerID).Scan(&until)
	assert.True(t, until.Valid, "user should be frozen after rejection")

	// Now the customer should be blocked from posting another review (or note).
	// We use a ticket note since we can target the same customer's ticket.
	resp = doModJSON(t, custClient, http.MethodPost,
		srv.URL+"/api/v1/tickets/"+strconv.FormatUint(ticketID, 10)+"/notes",
		map[string]string{"content": "should be blocked"}, ccsrf)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)

	var errBody map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&errBody)
	errObj := errBody["error"].(map[string]interface{})
	assert.Equal(t, "posting_frozen", errObj["code"])
}

func TestHTTP_ListQueue_NonModerator_403(t *testing.T) {
	srv, db := modServer(t)
	seedRoleUser(t, db, "regularz", "rz@t.l", "regular_user")

	_ = db
	client := srv.Client()
	client.Jar = newModJar()
	loginMod(t, client, srv.URL, "regularz")

	resp := doModJSON(t, client, http.MethodGet, srv.URL+"/api/v1/moderation/queue", nil, "")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestHTTP_ListUserViolations_Admin(t *testing.T) {
	srv, db := modServer(t)
	seedRoleUser(t, db, "adminv", "av@t.l", "administrator")
	target := seedRoleUser(t, db, "violator", "v@t.l", "regular_user")

	db.Exec(`INSERT INTO violation_records (user_id, content_type, content_id, freeze_applied, freeze_duration_hours)
		VALUES (?,?,?,?,?)`, target, "review", 1, true, 24)

	client := srv.Client()
	client.Jar = newModJar()
	loginMod(t, client, srv.URL, "adminv")

	resp := doModJSON(t, client, http.MethodGet,
		srv.URL+"/api/v1/admin/users/"+strconv.FormatUint(target, 10)+"/violations", nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	violations := body["violations"].([]interface{})
	assert.Len(t, violations, 1)
}
