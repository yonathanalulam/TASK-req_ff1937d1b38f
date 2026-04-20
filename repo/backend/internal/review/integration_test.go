package review_test

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

func reviewServerWithDB(t *testing.T) (*httptest.Server, *sql.DB) {
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
		AppEnv: "test", Port: "8080", DBHost: "db", DBPort: "3306",
		FieldEncryptionKey: "", SessionCookieDomain: "localhost",
	}
	r := router.New(cfg, db)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, db
}

func doReviewJSON(t *testing.T, client *http.Client, method, url string, body any, csrf string) *http.Response {
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

type reviewJar struct{ m map[string][]*http.Cookie }

func newReviewJar() *reviewJar                                      { return &reviewJar{m: make(map[string][]*http.Cookie)} }
func (j *reviewJar) SetCookies(u *url.URL, c []*http.Cookie)        { j.m[u.Host] = append(j.m[u.Host], c...) }
func (j *reviewJar) Cookies(u *url.URL) []*http.Cookie              { return j.m[u.Host] }

func seedUser(t *testing.T, db *sql.DB, username, email, role string) uint64 {
	t.Helper()
	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		username, email, seedPwHash, username)
	var id uint64
	db.QueryRow(`SELECT id FROM users WHERE username=?`, username).Scan(&id)
	db.Exec(`INSERT INTO user_roles (user_id, role_id) SELECT ?, id FROM roles WHERE name = ?`, id, role)
	return id
}

func loginUser(t *testing.T, client *http.Client, base, username string) string {
	t.Helper()
	resp := doReviewJSON(t, client, http.MethodPost, base+"/api/v1/auth/login",
		map[string]string{"username": username, "password": "password"}, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	return body["csrf_token"].(string)
}

// seedCompletedTicket creates a completed ticket owned by userID and returns (offeringID, ticketID).
func seedCompletedTicket(t *testing.T, db *sql.DB, agentID, userID uint64) (uint64, uint64) {
	t.Helper()
	db.Exec(`INSERT INTO service_categories (name, slug, response_time_minutes) VALUES (?,?,?)`, "RHC", "rhc", 60)
	var catID uint64
	db.QueryRow(`SELECT id FROM service_categories WHERE slug='rhc'`).Scan(&catID)
	db.Exec(`INSERT INTO service_offerings (agent_id, category_id, name, duration_minutes) VALUES (?,?,?,?)`,
		agentID, catID, "RHO", 60)
	var offID uint64
	db.QueryRow(`SELECT id FROM service_offerings WHERE name='RHO'`).Scan(&offID)
	db.Exec(`INSERT INTO addresses (user_id, label, address_line1_encrypted, city, state, zip, is_default)
			VALUES (?,?,?,?,?,?,1)`, userID, "H", []byte("1 Main"), "X", "NY", "10001")
	var addrID uint64
	db.QueryRow(`SELECT id FROM addresses WHERE user_id=?`, userID).Scan(&addrID)

	db.Exec(`INSERT INTO tickets (user_id, offering_id, category_id, address_id, preferred_start, preferred_end,
			delivery_method, status) VALUES (?,?,?,?,'2026-06-01 10:00:00','2026-06-01 12:00:00','pickup','Completed')`,
		userID, offID, catID, addrID)
	var ticketID uint64
	db.QueryRow(`SELECT id FROM tickets WHERE user_id=?`, userID).Scan(&ticketID)
	return offID, ticketID
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestHTTP_CreateReview_Success(t *testing.T) {
	srv, db := reviewServerWithDB(t)

	agentID := seedUser(t, db, "rva", "rva@t.l", "service_agent")
	userID := seedUser(t, db, "rvu", "rvu@t.l", "regular_user")
	_, ticketID := seedCompletedTicket(t, db, agentID, userID)

	client := srv.Client()
	client.Jar = newReviewJar()
	csrf := loginUser(t, client, srv.URL, "rvu")

	resp := doReviewJSON(t, client, http.MethodPost,
		srv.URL+"/api/v1/tickets/"+strconv.FormatUint(ticketID, 10)+"/reviews",
		map[string]interface{}{"rating": 5, "text": "Great work!"}, csrf)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	r := body["review"].(map[string]interface{})
	assert.Equal(t, float64(5), r["rating"])
	assert.Equal(t, "Great work!", r["text"])
}

func TestHTTP_CreateReview_DuplicateReturns409(t *testing.T) {
	srv, db := reviewServerWithDB(t)

	agentID := seedUser(t, db, "dpa", "dpa@t.l", "service_agent")
	userID := seedUser(t, db, "dpu", "dpu@t.l", "regular_user")
	_, ticketID := seedCompletedTicket(t, db, agentID, userID)

	client := srv.Client()
	client.Jar = newReviewJar()
	csrf := loginUser(t, client, srv.URL, "dpu")

	url := srv.URL + "/api/v1/tickets/" + strconv.FormatUint(ticketID, 10) + "/reviews"
	resp := doReviewJSON(t, client, http.MethodPost, url,
		map[string]interface{}{"rating": 4}, csrf)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	resp = doReviewJSON(t, client, http.MethodPost, url,
		map[string]interface{}{"rating": 3}, csrf)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestHTTP_CreateReview_OnAcceptedTicket_422(t *testing.T) {
	srv, db := reviewServerWithDB(t)

	agentID := seedUser(t, db, "eaa", "eaa@t.l", "service_agent")
	userID := seedUser(t, db, "eau", "eau@t.l", "regular_user")
	// Seed ticket in Accepted (not eligible)
	db.Exec(`INSERT INTO service_categories (name, slug, response_time_minutes) VALUES (?,?,?)`, "EC", "ec", 60)
	var catID uint64
	db.QueryRow(`SELECT id FROM service_categories WHERE slug='ec'`).Scan(&catID)
	db.Exec(`INSERT INTO service_offerings (agent_id, category_id, name, duration_minutes) VALUES (?,?,?,?)`,
		agentID, catID, "EO", 60)
	var offID uint64
	db.QueryRow(`SELECT id FROM service_offerings WHERE name='EO'`).Scan(&offID)
	db.Exec(`INSERT INTO addresses (user_id, label, address_line1_encrypted, city, state, zip, is_default)
			VALUES (?,?,?,?,?,?,1)`, userID, "H", []byte("1"), "X", "NY", "10001")
	var addrID uint64
	db.QueryRow(`SELECT id FROM addresses WHERE user_id=?`, userID).Scan(&addrID)
	db.Exec(`INSERT INTO tickets (user_id, offering_id, category_id, address_id, preferred_start, preferred_end,
			delivery_method, status) VALUES (?,?,?,?,'2026-06-01 10:00:00','2026-06-01 12:00:00','pickup','Accepted')`,
		userID, offID, catID, addrID)
	var ticketID uint64
	db.QueryRow(`SELECT id FROM tickets WHERE user_id=?`, userID).Scan(&ticketID)

	client := srv.Client()
	client.Jar = newReviewJar()
	csrf := loginUser(t, client, srv.URL, "eau")

	resp := doReviewJSON(t, client, http.MethodPost,
		srv.URL+"/api/v1/tickets/"+strconv.FormatUint(ticketID, 10)+"/reviews",
		map[string]interface{}{"rating": 5}, csrf)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

func TestHTTP_UpdateReview_Owner(t *testing.T) {
	srv, db := reviewServerWithDB(t)

	agentID := seedUser(t, db, "uva", "uva@t.l", "service_agent")
	userID := seedUser(t, db, "uvu", "uvu@t.l", "regular_user")
	_, ticketID := seedCompletedTicket(t, db, agentID, userID)

	client := srv.Client()
	client.Jar = newReviewJar()
	csrf := loginUser(t, client, srv.URL, "uvu")

	// Create
	resp := doReviewJSON(t, client, http.MethodPost,
		srv.URL+"/api/v1/tickets/"+strconv.FormatUint(ticketID, 10)+"/reviews",
		map[string]interface{}{"rating": 3, "text": "Meh."}, csrf)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var createdBody map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&createdBody)
	resp.Body.Close()
	reviewID := uint64(createdBody["review"].(map[string]interface{})["id"].(float64))

	// Update
	url := srv.URL + "/api/v1/tickets/" + strconv.FormatUint(ticketID, 10) +
		"/reviews/" + strconv.FormatUint(reviewID, 10)
	resp = doReviewJSON(t, client, http.MethodPut, url,
		map[string]interface{}{"rating": 5, "text": "Actually excellent"}, csrf)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	r := body["review"].(map[string]interface{})
	assert.Equal(t, float64(5), r["rating"])
	assert.Equal(t, "Actually excellent", r["text"])
}

func TestHTTP_ListReviews_Public(t *testing.T) {
	srv, db := reviewServerWithDB(t)

	agentID := seedUser(t, db, "lra", "lra@t.l", "service_agent")
	userID := seedUser(t, db, "lru", "lru@t.l", "regular_user")
	offID, ticketID := seedCompletedTicket(t, db, agentID, userID)

	db.Exec(`INSERT INTO reviews (ticket_id, user_id, offering_id, rating, text, status)
			VALUES (?,?,?,?,?, 'published')`,
		ticketID, userID, offID, 4, "solid")

	// Unauthenticated — reviews are public
	client := srv.Client()
	resp := doReviewJSON(t, client, http.MethodGet,
		srv.URL+"/api/v1/service-offerings/"+strconv.FormatUint(offID, 10)+"/reviews", nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()

	items := body["items"].([]interface{})
	assert.Len(t, items, 1)
	assert.Equal(t, float64(4), items[0].(map[string]interface{})["rating"])
}

func TestHTTP_Summary_Public(t *testing.T) {
	srv, db := reviewServerWithDB(t)

	agentID := seedUser(t, db, "sma", "sma@t.l", "service_agent")
	userID := seedUser(t, db, "smu", "smu@t.l", "regular_user")
	offID, ticketID := seedCompletedTicket(t, db, agentID, userID)

	db.Exec(`INSERT INTO reviews (ticket_id, user_id, offering_id, rating, status) VALUES (?,?,?,?, 'published')`,
		ticketID, userID, offID, 5)

	client := srv.Client()
	resp := doReviewJSON(t, client, http.MethodGet,
		srv.URL+"/api/v1/service-offerings/"+strconv.FormatUint(offID, 10)+"/review-summary", nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var sum map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&sum)
	resp.Body.Close()

	assert.Equal(t, float64(1), sum["total_reviews"])
	assert.Equal(t, 5.0, sum["average_rating"])
	assert.Equal(t, 1.0, sum["positive_rate"])
}

func TestHTTP_ReportReview_Created(t *testing.T) {
	srv, db := reviewServerWithDB(t)

	agentID := seedUser(t, db, "rpa", "rpa@t.l", "service_agent")
	authorID := seedUser(t, db, "rpau", "rpau@t.l", "regular_user")
	reporterID := seedUser(t, db, "rpr", "rpr@t.l", "regular_user")
	offID, ticketID := seedCompletedTicket(t, db, agentID, authorID)

	db.Exec(`INSERT INTO reviews (ticket_id, user_id, offering_id, rating, text, status)
			VALUES (?,?,?,?,?, 'published')`,
		ticketID, authorID, offID, 1, "not useful")
	var reviewID uint64
	db.QueryRow(`SELECT id FROM reviews WHERE ticket_id=?`, ticketID).Scan(&reviewID)

	client := srv.Client()
	client.Jar = newReviewJar()
	csrf := loginUser(t, client, srv.URL, "rpr")
	_ = reporterID

	resp := doReviewJSON(t, client, http.MethodPost,
		srv.URL+"/api/v1/reviews/"+strconv.FormatUint(reviewID, 10)+"/reports",
		map[string]string{"reason": "spam", "details": "keyword stuffing"}, csrf)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	var n int
	db.QueryRow(`SELECT COUNT(*) FROM review_reports WHERE review_id=?`, reviewID).Scan(&n)
	assert.Equal(t, 1, n)
}

func TestHTTP_ReportReview_InvalidReason_422(t *testing.T) {
	srv, db := reviewServerWithDB(t)

	agentID := seedUser(t, db, "iva", "iva@t.l", "service_agent")
	authorID := seedUser(t, db, "ivau", "ivau@t.l", "regular_user")
	reporterID := seedUser(t, db, "ivr", "ivr@t.l", "regular_user")
	_ = reporterID
	offID, ticketID := seedCompletedTicket(t, db, agentID, authorID)
	db.Exec(`INSERT INTO reviews (ticket_id, user_id, offering_id, rating, status) VALUES (?,?,?,?, 'published')`,
		ticketID, authorID, offID, 2)
	var reviewID uint64
	db.QueryRow(`SELECT id FROM reviews WHERE ticket_id=?`, ticketID).Scan(&reviewID)

	client := srv.Client()
	client.Jar = newReviewJar()
	csrf := loginUser(t, client, srv.URL, "ivr")

	resp := doReviewJSON(t, client, http.MethodPost,
		srv.URL+"/api/v1/reviews/"+strconv.FormatUint(reviewID, 10)+"/reports",
		map[string]string{"reason": "notvalid"}, csrf)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}
