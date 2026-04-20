package qa_test

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

func qaServerWithDB(t *testing.T) (*httptest.Server, *sql.DB) {
	t.Helper()
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"qa_posts", "qa_threads",
		"review_reports", "review_images", "reviews",
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

func doQAJSON(t *testing.T, client *http.Client, method, url string, body any, csrf string) *http.Response {
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

type qaJar struct{ m map[string][]*http.Cookie }

func newQAJar() *qaJar                                      { return &qaJar{m: make(map[string][]*http.Cookie)} }
func (j *qaJar) SetCookies(u *url.URL, c []*http.Cookie)    { j.m[u.Host] = append(j.m[u.Host], c...) }
func (j *qaJar) Cookies(u *url.URL) []*http.Cookie          { return j.m[u.Host] }

func seedQAUser(t *testing.T, db *sql.DB, username, email, role string) uint64 {
	t.Helper()
	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		username, email, seedPwHash, username)
	var id uint64
	db.QueryRow(`SELECT id FROM users WHERE username=?`, username).Scan(&id)
	db.Exec(`INSERT INTO user_roles (user_id, role_id) SELECT ?, id FROM roles WHERE name = ?`, id, role)
	return id
}

func loginQAUser(t *testing.T, client *http.Client, base, username string) string {
	t.Helper()
	resp := doQAJSON(t, client, http.MethodPost, base+"/api/v1/auth/login",
		map[string]string{"username": username, "password": "password"}, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	return body["csrf_token"].(string)
}

func seedOffering(t *testing.T, db *sql.DB, agentID uint64) uint64 {
	t.Helper()
	db.Exec(`INSERT INTO service_categories (name, slug, response_time_minutes) VALUES (?,?,?)`, "QC", "qhc", 60)
	var catID uint64
	db.QueryRow(`SELECT id FROM service_categories WHERE slug='qhc'`).Scan(&catID)
	db.Exec(`INSERT INTO service_offerings (agent_id, category_id, name, duration_minutes) VALUES (?,?,?,?)`,
		agentID, catID, "QOff", 60)
	var offID uint64
	db.QueryRow(`SELECT id FROM service_offerings WHERE name='QOff'`).Scan(&offID)
	return offID
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestHTTP_QA_RegularUserCreatesQuestion(t *testing.T) {
	srv, db := qaServerWithDB(t)

	agentID := seedQAUser(t, db, "qaag", "qa@t.l", "service_agent")
	userID := seedQAUser(t, db, "qau", "qau@t.l", "regular_user")
	_ = userID
	offID := seedOffering(t, db, agentID)

	client := srv.Client()
	client.Jar = newQAJar()
	csrf := loginQAUser(t, client, srv.URL, "qau")

	resp := doQAJSON(t, client, http.MethodPost,
		srv.URL+"/api/v1/service-offerings/"+strconv.FormatUint(offID, 10)+"/qa",
		map[string]string{"question": "What are your hours?"}, csrf)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	th := body["thread"].(map[string]interface{})
	assert.Equal(t, "What are your hours?", th["question"])
}

func TestHTTP_QA_ServiceAgentReplies(t *testing.T) {
	srv, db := qaServerWithDB(t)

	agentID := seedQAUser(t, db, "rpaag", "rpaag@t.l", "service_agent")
	userID := seedQAUser(t, db, "rpau", "rpau@t.l", "regular_user")
	offID := seedOffering(t, db, agentID)

	// Create a thread directly
	db.Exec(`INSERT INTO qa_threads (offering_id, author_id, question, status) VALUES (?,?,?, 'published')`,
		offID, userID, "Do you do weekends?")
	var threadID uint64
	db.QueryRow(`SELECT id FROM qa_threads WHERE offering_id=?`, offID).Scan(&threadID)

	// Service agent logs in and replies
	client := srv.Client()
	client.Jar = newQAJar()
	csrf := loginQAUser(t, client, srv.URL, "rpaag")

	resp := doQAJSON(t, client, http.MethodPost,
		srv.URL+"/api/v1/service-offerings/"+strconv.FormatUint(offID, 10)+
			"/qa/"+strconv.FormatUint(threadID, 10)+"/replies",
		map[string]string{"content": "Yes, with 24-hour notice."}, csrf)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()
}

func TestHTTP_QA_RegularUserCannotReply_403(t *testing.T) {
	srv, db := qaServerWithDB(t)

	agentID := seedQAUser(t, db, "nrag", "nrag@t.l", "service_agent")
	userID := seedQAUser(t, db, "nru", "nru@t.l", "regular_user")
	offID := seedOffering(t, db, agentID)

	db.Exec(`INSERT INTO qa_threads (offering_id, author_id, question, status) VALUES (?,?,?, 'published')`,
		offID, userID, "Question?")
	var threadID uint64
	db.QueryRow(`SELECT id FROM qa_threads WHERE offering_id=?`, offID).Scan(&threadID)

	client := srv.Client()
	client.Jar = newQAJar()
	csrf := loginQAUser(t, client, srv.URL, "nru")

	resp := doQAJSON(t, client, http.MethodPost,
		srv.URL+"/api/v1/service-offerings/"+strconv.FormatUint(offID, 10)+
			"/qa/"+strconv.FormatUint(threadID, 10)+"/replies",
		map[string]string{"content": "I'll reply even though I shouldn't"}, csrf)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestHTTP_QA_ListThreads(t *testing.T) {
	srv, db := qaServerWithDB(t)

	agentID := seedQAUser(t, db, "ltag", "ltag@t.l", "service_agent")
	userID := seedQAUser(t, db, "ltu", "ltu@t.l", "regular_user")
	offID := seedOffering(t, db, agentID)

	for i := 0; i < 3; i++ {
		db.Exec(`INSERT INTO qa_threads (offering_id, author_id, question, status) VALUES (?,?,?, 'published')`,
			offID, userID, "q?")
	}

	client := srv.Client()
	client.Jar = newQAJar()
	loginQAUser(t, client, srv.URL, "ltu")

	resp := doQAJSON(t, client, http.MethodGet,
		srv.URL+"/api/v1/service-offerings/"+strconv.FormatUint(offID, 10)+"/qa", nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	assert.Len(t, body["items"].([]interface{}), 3)
}

func TestHTTP_QA_ModeratorDeletesPost(t *testing.T) {
	srv, db := qaServerWithDB(t)

	agentID := seedQAUser(t, db, "mdag", "mdag@t.l", "service_agent")
	userID := seedQAUser(t, db, "mdu", "mdu@t.l", "regular_user")
	modID := seedQAUser(t, db, "moderator", "mod@t.l", "moderator")
	_ = modID
	offID := seedOffering(t, db, agentID)

	db.Exec(`INSERT INTO qa_threads (offering_id, author_id, question, status) VALUES (?,?,?, 'published')`,
		offID, userID, "q?")
	var threadID uint64
	db.QueryRow(`SELECT id FROM qa_threads WHERE offering_id=?`, offID).Scan(&threadID)
	db.Exec(`INSERT INTO qa_posts (thread_id, author_id, content, status) VALUES (?,?,?, 'published')`,
		threadID, agentID, "reply content")
	var postID uint64
	db.QueryRow(`SELECT id FROM qa_posts WHERE thread_id=?`, threadID).Scan(&postID)

	client := srv.Client()
	client.Jar = newQAJar()
	csrf := loginQAUser(t, client, srv.URL, "moderator")

	resp := doQAJSON(t, client, http.MethodDelete,
		srv.URL+"/api/v1/qa/"+strconv.FormatUint(postID, 10), nil, csrf)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Verify the reply no longer appears in thread
	client2 := srv.Client()
	client2.Jar = newQAJar()
	loginQAUser(t, client2, srv.URL, "mdu")
	resp2 := doQAJSON(t, client2, http.MethodGet,
		srv.URL+"/api/v1/service-offerings/"+strconv.FormatUint(offID, 10)+"/qa", nil, "")
	var body map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&body)
	resp2.Body.Close()
	threads := body["items"].([]interface{})
	require.Len(t, threads, 1)
	th := threads[0].(map[string]interface{})
	// "replies" key missing or empty
	replies, ok := th["replies"].([]interface{})
	if ok {
		assert.Len(t, replies, 0)
	}
}

func TestHTTP_QA_RegularUserCannotDelete_403(t *testing.T) {
	srv, db := qaServerWithDB(t)

	agentID := seedQAUser(t, db, "rdag", "rdag@t.l", "service_agent")
	userID := seedQAUser(t, db, "rdu", "rdu@t.l", "regular_user")
	offID := seedOffering(t, db, agentID)

	db.Exec(`INSERT INTO qa_threads (offering_id, author_id, question, status) VALUES (?,?,?, 'published')`,
		offID, userID, "q?")
	var threadID uint64
	db.QueryRow(`SELECT id FROM qa_threads WHERE offering_id=?`, offID).Scan(&threadID)
	db.Exec(`INSERT INTO qa_posts (thread_id, author_id, content, status) VALUES (?,?,?, 'published')`,
		threadID, agentID, "content")
	var postID uint64
	db.QueryRow(`SELECT id FROM qa_posts WHERE thread_id=?`, threadID).Scan(&postID)

	client := srv.Client()
	client.Jar = newQAJar()
	csrf := loginQAUser(t, client, srv.URL, "rdu")

	resp := doQAJSON(t, client, http.MethodDelete,
		srv.URL+"/api/v1/qa/"+strconv.FormatUint(postID, 10), nil, csrf)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}
