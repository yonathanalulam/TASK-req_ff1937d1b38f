package notification_test

import (
	"bytes"
	"context"
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
	"github.com/eagle-point/service-portal/internal/notification"
	"github.com/eagle-point/service-portal/internal/router"
	"github.com/eagle-point/service-portal/internal/testutil"
)

const seedPwHash = "$2a$10$92IXUNpkjO0rOQ5byMi.Ye4oKoEa3Ro9llC/.og/at2.uheWG/igi"

func notifServer(t *testing.T) (*httptest.Server, *sql.DB) {
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

func doNotifJSON(t *testing.T, client *http.Client, method, url string, body any, csrf string) *http.Response {
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

type notifJar struct{ m map[string][]*http.Cookie }

func newNotifJar() *notifJar                                  { return &notifJar{m: make(map[string][]*http.Cookie)} }
func (j *notifJar) SetCookies(u *url.URL, c []*http.Cookie)   { j.m[u.Host] = append(j.m[u.Host], c...) }
func (j *notifJar) Cookies(u *url.URL) []*http.Cookie         { return j.m[u.Host] }

func seedNotifUser(t *testing.T, db *sql.DB, username, email string, notifyInApp bool) uint64 {
	t.Helper()
	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		username, email, seedPwHash, username)
	var id uint64
	db.QueryRow(`SELECT id FROM users WHERE username=?`, username).Scan(&id)
	db.Exec(`INSERT INTO user_roles (user_id, role_id) SELECT ?, id FROM roles WHERE name='regular_user'`, id)
	db.Exec(`INSERT INTO user_preferences (user_id, notify_in_app) VALUES (?, ?)`, id, notifyInApp)
	return id
}

func loginNotif(t *testing.T, client *http.Client, base, username string) string {
	t.Helper()
	resp := doNotifJSON(t, client, http.MethodPost, base+"/api/v1/auth/login",
		map[string]string{"username": username, "password": "password"}, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	return body["csrf_token"].(string)
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestHTTP_AuthMe_IncludesUnreadCount(t *testing.T) {
	srv, db := notifServer(t)
	userID := seedNotifUser(t, db, "memetest", "me@t.l", true)

	// Insert a notification directly
	svc := notification.NewService(db)
	svc.UpsertTemplate(context.Background(), "memetest", "T", "B")
	_, err := svc.Dispatch(context.Background(), userID, "memetest", nil)
	require.NoError(t, err)

	client := srv.Client()
	client.Jar = newNotifJar()
	loginNotif(t, client, srv.URL, "memetest")

	resp := doNotifJSON(t, client, http.MethodGet, srv.URL+"/api/v1/auth/me", nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()

	assert.Equal(t, float64(1), body["unread_count"])
}

func TestHTTP_ListNotifications_RequiresAuth(t *testing.T) {
	srv, _ := notifServer(t)
	client := srv.Client()
	resp := doNotifJSON(t, client, http.MethodGet, srv.URL+"/api/v1/users/me/notifications", nil, "")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestHTTP_ListAndMarkReadFlow(t *testing.T) {
	srv, db := notifServer(t)
	userID := seedNotifUser(t, db, "rwflow", "rw@t.l", true)

	svc := notification.NewService(db)
	svc.UpsertTemplate(context.Background(), "rw", "T", "B")
	for i := 0; i < 3; i++ {
		svc.Dispatch(context.Background(), userID, "rw", nil)
	}

	client := srv.Client()
	client.Jar = newNotifJar()
	csrf := loginNotif(t, client, srv.URL, "rwflow")

	// List
	resp := doNotifJSON(t, client, http.MethodGet, srv.URL+"/api/v1/users/me/notifications", nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var page map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&page)
	resp.Body.Close()
	items := page["items"].([]interface{})
	require.Len(t, items, 3)

	// Mark first as read
	firstID := uint64(items[0].(map[string]interface{})["id"].(float64))
	resp = doNotifJSON(t, client, http.MethodPatch,
		srv.URL+"/api/v1/users/me/notifications/"+strconv.FormatUint(firstID, 10)+"/read", nil, csrf)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	// Unread count via /auth/me
	resp = doNotifJSON(t, client, http.MethodGet, srv.URL+"/api/v1/auth/me", nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var meBody map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&meBody)
	resp.Body.Close()
	assert.Equal(t, float64(2), meBody["unread_count"])

	// Mark all read
	resp = doNotifJSON(t, client, http.MethodPatch,
		srv.URL+"/api/v1/users/me/notifications/read-all", nil, csrf)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Unread is now 0
	resp = doNotifJSON(t, client, http.MethodGet, srv.URL+"/api/v1/auth/me", nil, "")
	json.NewDecoder(resp.Body).Decode(&meBody)
	resp.Body.Close()
	assert.Equal(t, float64(0), meBody["unread_count"])
}

func TestHTTP_Outbox_ReceivesWhenInAppDisabled(t *testing.T) {
	srv, db := notifServer(t)
	userID := seedNotifUser(t, db, "outboxer", "ob@t.l", false)

	svc := notification.NewService(db)
	svc.UpsertTemplate(context.Background(), "ox", "T", "B")
	svc.Dispatch(context.Background(), userID, "ox", nil)

	client := srv.Client()
	client.Jar = newNotifJar()
	loginNotif(t, client, srv.URL, "outboxer")

	resp := doNotifJSON(t, client, http.MethodGet,
		srv.URL+"/api/v1/users/me/notifications/outbox", nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	items := body["items"].([]interface{})
	assert.Len(t, items, 1)
}

func TestHTTP_TicketStatusChange_DispatchesNotification(t *testing.T) {
	srv, db := notifServer(t)

	// Seed agent + customer
	userID := seedNotifUser(t, db, "ticketcustomer", "tc@t.l", true)
	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"sa1", "sa1@t.l", seedPwHash, "Sa1")
	var agentID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='sa1'`).Scan(&agentID)
	db.Exec(`INSERT INTO user_roles (user_id, role_id) SELECT ?, id FROM roles WHERE name='service_agent'`, agentID)

	db.Exec(`INSERT INTO service_categories (name, slug, response_time_minutes) VALUES (?,?,?)`, "TC", "tc", 60)
	var catID uint64
	db.QueryRow(`SELECT id FROM service_categories WHERE slug='tc'`).Scan(&catID)
	db.Exec(`INSERT INTO service_offerings (agent_id, category_id, name, duration_minutes) VALUES (?,?,?,?)`,
		agentID, catID, "TO", 60)
	var offID uint64
	db.QueryRow(`SELECT id FROM service_offerings WHERE name='TO'`).Scan(&offID)
	db.Exec(`INSERT INTO addresses (user_id, label, address_line1_encrypted, city, state, zip, is_default) VALUES (?,?,?,?,?,?,1)`,
		userID, "H", []byte("1 Main"), "X", "NY", "10001")
	var addrID uint64
	db.QueryRow(`SELECT id FROM addresses WHERE user_id=?`, userID).Scan(&addrID)

	// Insert a ticket directly in Accepted
	db.Exec(`INSERT INTO tickets (user_id, offering_id, category_id, address_id, preferred_start, preferred_end,
		delivery_method, status) VALUES (?,?,?,?,'2026-06-01 10:00:00','2026-06-01 12:00:00','pickup','Accepted')`,
		userID, offID, catID, addrID)
	var ticketID uint64
	db.QueryRow(`SELECT id FROM tickets WHERE user_id=?`, userID).Scan(&ticketID)

	// Agent transitions to Dispatched via HTTP
	client := srv.Client()
	client.Jar = newNotifJar()
	csrf := loginNotif(t, client, srv.URL, "sa1")
	resp := doNotifJSON(t, client, http.MethodPatch,
		srv.URL+"/api/v1/tickets/"+strconv.FormatUint(ticketID, 10)+"/status",
		map[string]string{"status": "Dispatched"}, csrf)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Customer should now have a notification
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE user_id=? AND template_code='ticket_status_change'`, userID).Scan(&n)
	assert.Equal(t, 1, n, "ticket status change should dispatch a notification to the owner")
}

func TestHTTP_AdminUpsertTemplate(t *testing.T) {
	srv, db := notifServer(t)

	// Seed admin user
	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"adminx", "ax@t.l", seedPwHash, "Adminx")
	var adminID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='adminx'`).Scan(&adminID)
	db.Exec(`INSERT INTO user_roles (user_id, role_id) SELECT ?, id FROM roles WHERE name='administrator'`, adminID)
	db.Exec(`INSERT INTO user_preferences (user_id, notify_in_app) VALUES (?, 1)`, adminID)

	client := srv.Client()
	client.Jar = newNotifJar()
	csrf := loginNotif(t, client, srv.URL, "adminx")

	resp := doNotifJSON(t, client, http.MethodPut,
		srv.URL+"/api/v1/admin/notification-templates/custom_event",
		map[string]string{
			"title_template": "Custom: {{.Name}}",
			"body_template":  "Hello {{.Name}}",
		}, csrf)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Verify it persisted
	var title string
	db.QueryRow(`SELECT title_template FROM notification_templates WHERE code='custom_event'`).Scan(&title)
	assert.Equal(t, "Custom: {{.Name}}", title)
}
