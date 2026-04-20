package securitytest_test

// Object-level authorization (IDOR) sweep.
//
// For every resource that exposes an :id URL parameter belonging to a user,
// we verify: a second user cannot GET, PUT, DELETE, or otherwise mutate it.
// Plus a mass-assignment probe: POST bodies with extra fields like user_id
// must not be able to re-target ownership on create.

import (
	"database/sql"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedAddressFor inserts a stored address for userID and returns its id.
// Uses raw SQL so the test is independent of the address service's public
// API — we want to verify the handler layer rejects cross-user access even
// when a real row exists in the DB.
func seedAddressFor(t *testing.T, db *sql.DB, userID uint64, label string) uint64 {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO addresses (user_id, label, address_line1_encrypted, city, state, zip, is_default)
		 VALUES (?, ?, ?, ?, ?, ?, 1)`,
		userID, label, []byte("123 Main"), "Townville", "NY", "10001")
	require.NoError(t, err)
	return scanUintFromQuery(t, db, `SELECT id FROM addresses WHERE user_id=? AND label=?`, userID, label)
}

// seedTicketFor inserts a Completed ticket for userID and returns its id.
// Completed is convenient because it allows review creation for later tests
// without needing to simulate SLA transitions.
func seedTicketFor(t *testing.T, db *sql.DB, userID uint64) (ticketID, offeringID, categoryID, addressID uint64) {
	t.Helper()
	_, _ = db.Exec(`INSERT INTO service_categories (name, slug, response_time_minutes) VALUES (?, ?, ?)`,
		"SecCat", "seccat", 60)
	categoryID = scanUintFromQuery(t, db, `SELECT id FROM service_categories WHERE slug=?`, "seccat")

	_, _ = db.Exec(`INSERT INTO service_offerings (agent_id, category_id, name, duration_minutes) VALUES (?, ?, ?, ?)`,
		userID, categoryID, "SecOff", 60)
	offeringID = scanUintFromQuery(t, db, `SELECT id FROM service_offerings WHERE name=?`, "SecOff")

	addressID = seedAddressFor(t, db, userID, "primary")

	_, _ = db.Exec(`INSERT INTO tickets
		(user_id, offering_id, category_id, address_id, preferred_start, preferred_end,
		 delivery_method, status)
		VALUES (?, ?, ?, ?, ?, ?, 'pickup', 'Completed')`,
		userID, offeringID, categoryID, addressID,
		time.Now().Add(24*time.Hour), time.Now().Add(26*time.Hour))
	ticketID = scanUintFromQuery(t, db, `SELECT id FROM tickets WHERE user_id=? AND offering_id=?`,
		userID, offeringID)
	return
}

// seedNotificationFor inserts a notification owned by userID and returns id.
func seedNotificationFor(t *testing.T, db *sql.DB, userID uint64) uint64 {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO notifications (user_id, template_code, title, body, is_read) VALUES (?, ?, ?, ?, 0)`,
		userID, "test.notif", "title", "body")
	require.NoError(t, err)
	return scanUintFromQuery(t, db, `SELECT id FROM notifications WHERE user_id=? ORDER BY id DESC LIMIT 1`, userID)
}

// seedReviewFor inserts a published review by userID on ticketID/offeringID.
func seedReviewFor(t *testing.T, db *sql.DB, userID, offeringID, ticketID uint64) uint64 {
	t.Helper()
	_, err := db.Exec(`INSERT INTO reviews
		(user_id, offering_id, ticket_id, rating, text, status)
		VALUES (?, ?, ?, ?, ?, 'published')`,
		userID, offeringID, ticketID, 5, "good service")
	require.NoError(t, err)
	return scanUintFromQuery(t, db, `SELECT id FROM reviews WHERE user_id=? AND ticket_id=?`, userID, ticketID)
}

// ─── Addresses ───────────────────────────────────────────────────────────────

func TestIDOR_Address_CrossUserUpdateBlocked(t *testing.T) {
	srv, db := securityServer(t)
	alice := seedUser(t, db, "alice_addr", "regular_user")
	_ = alice
	bob := seedUser(t, db, "bob_addr", "regular_user")
	bobAddr := seedAddressFor(t, db, bob, "bob-primary")

	aliceCli := newClient(t, srv)
	aliceCSRF := loginAs(t, aliceCli, srv.URL, "alice_addr")

	// Alice tries to PUT Bob's address id.
	resp := doJSON(t, aliceCli, http.MethodPut,
		srv.URL+"/api/v1/users/me/addresses/"+u64Str(bobAddr),
		aliceCSRF,
		map[string]any{
			"label":          "hijacked",
			"address_line1":  "evil 1",
			"city":           "X", "state": "NY", "zip": "10001",
		})
	defer resp.Body.Close()

	// Correct behaviour: 404 (address doesn't exist "for Alice") — better than
	// 403 because it doesn't confirm the id exists for someone else.
	assert.NotEqual(t, http.StatusOK, resp.StatusCode,
		"cross-user update must NOT succeed")
	assert.Contains(t, []int{http.StatusNotFound, http.StatusForbidden}, resp.StatusCode,
		"expected 403 or 404, got %d", resp.StatusCode)

	// Bob's address must be untouched.
	var label string
	require.NoError(t, db.QueryRow(`SELECT label FROM addresses WHERE id=?`, bobAddr).Scan(&label))
	assert.Equal(t, "bob-primary", label,
		"Bob's address label must not be modified by Alice's request")
}

func TestIDOR_Address_CrossUserDeleteBlocked(t *testing.T) {
	srv, db := securityServer(t)
	_ = seedUser(t, db, "alice_del", "regular_user")
	bob := seedUser(t, db, "bob_del", "regular_user")
	bobAddr := seedAddressFor(t, db, bob, "bob-asset")

	aliceCli := newClient(t, srv)
	aliceCSRF := loginAs(t, aliceCli, srv.URL, "alice_del")

	resp := doJSON(t, aliceCli, http.MethodDelete,
		srv.URL+"/api/v1/users/me/addresses/"+u64Str(bobAddr), aliceCSRF, nil)
	resp.Body.Close()
	assert.NotEqual(t, http.StatusNoContent, resp.StatusCode,
		"Alice must not be able to delete Bob's address")

	// Bob's row still present.
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM addresses WHERE id=?`, bobAddr).Scan(&n)
	assert.Equal(t, 1, n, "Bob's address must still exist")
}

// ─── Tickets ─────────────────────────────────────────────────────────────────

func TestIDOR_Ticket_CrossUserReadBlocked(t *testing.T) {
	srv, db := securityServer(t)
	_ = seedUser(t, db, "alice_tkt", "regular_user")
	bob := seedUser(t, db, "bob_tkt", "regular_user")
	bobTicket, _, _, _ := seedTicketFor(t, db, bob)

	aliceCli := newClient(t, srv)
	aliceCSRF := loginAs(t, aliceCli, srv.URL, "alice_tkt")

	resp := doJSON(t, aliceCli, http.MethodGet,
		srv.URL+"/api/v1/tickets/"+u64Str(bobTicket), aliceCSRF, nil)
	resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode,
		"regular user must NOT read another user's ticket (CanView rejects)")
}

func TestIDOR_Ticket_ServiceAgentCanView(t *testing.T) {
	// Documents the intentional authorization model: service_agent may view
	// any ticket (scope broadens for operational triage). Captured as a test
	// so that tightening this rule later becomes a visible, explicit change.
	srv, db := securityServer(t)
	agent := seedUser(t, db, "agent_view", "service_agent")
	_ = agent
	customer := seedUser(t, db, "cust_view", "regular_user")
	custTicket, _, _, _ := seedTicketFor(t, db, customer)

	agentCli := newClient(t, srv)
	agentCSRF := loginAs(t, agentCli, srv.URL, "agent_view")

	resp := doJSON(t, agentCli, http.MethodGet,
		srv.URL+"/api/v1/tickets/"+u64Str(custTicket), agentCSRF, nil)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"service_agent may view any ticket by design — if this changes, update CanView")
}

func TestIDOR_Ticket_CrossUserCancelBlocked(t *testing.T) {
	srv, db := securityServer(t)
	_ = seedUser(t, db, "alice_can", "regular_user")
	bob := seedUser(t, db, "bob_can", "regular_user")
	bobTicket, _, _, _ := seedTicketFor(t, db, bob)
	// Force an in-flight status; cancel is only allowed pre-Dispatch in any
	// case, but the ownership check must fire before that logic.
	_, _ = db.Exec(`UPDATE tickets SET status='Accepted' WHERE id=?`, bobTicket)

	aliceCli := newClient(t, srv)
	aliceCSRF := loginAs(t, aliceCli, srv.URL, "alice_can")

	resp := doJSON(t, aliceCli, http.MethodPatch,
		srv.URL+"/api/v1/tickets/"+u64Str(bobTicket)+"/status",
		aliceCSRF,
		map[string]string{"status": "Cancelled", "cancel_reason": "hijack"})
	resp.Body.Close()
	assert.NotEqual(t, http.StatusOK, resp.StatusCode,
		"Alice must not cancel Bob's ticket")

	// Status must not have flipped.
	var status string
	db.QueryRow(`SELECT status FROM tickets WHERE id=?`, bobTicket).Scan(&status)
	assert.Equal(t, "Accepted", status, "Bob's ticket status must not be mutated by Alice")
}

// ─── Notifications ───────────────────────────────────────────────────────────

func TestIDOR_Notification_CrossUserMarkReadBlocked(t *testing.T) {
	srv, db := securityServer(t)
	_ = seedUser(t, db, "alice_notif", "regular_user")
	bob := seedUser(t, db, "bob_notif", "regular_user")
	bobNotif := seedNotificationFor(t, db, bob)

	aliceCli := newClient(t, srv)
	aliceCSRF := loginAs(t, aliceCli, srv.URL, "alice_notif")

	resp := doJSON(t, aliceCli, http.MethodPatch,
		srv.URL+"/api/v1/users/me/notifications/"+u64Str(bobNotif)+"/read",
		aliceCSRF, nil)
	resp.Body.Close()
	assert.NotEqual(t, http.StatusNoContent, resp.StatusCode,
		"Alice must not be able to mark Bob's notification as read")

	var isRead int
	db.QueryRow(`SELECT is_read FROM notifications WHERE id=?`, bobNotif).Scan(&isRead)
	assert.Equal(t, 0, isRead, "Bob's notification must remain unread")
}

// ─── Reviews ─────────────────────────────────────────────────────────────────

func TestIDOR_Review_CrossUserEditBlocked(t *testing.T) {
	srv, db := securityServer(t)
	_ = seedUser(t, db, "alice_rev", "regular_user")
	bob := seedUser(t, db, "bob_rev", "regular_user")
	bobTicket, bobOff, _, _ := seedTicketFor(t, db, bob)
	bobReview := seedReviewFor(t, db, bob, bobOff, bobTicket)

	aliceCli := newClient(t, srv)
	aliceCSRF := loginAs(t, aliceCli, srv.URL, "alice_rev")

	resp := doJSON(t, aliceCli, http.MethodPut,
		srv.URL+"/api/v1/tickets/"+u64Str(bobTicket)+"/reviews/"+u64Str(bobReview),
		aliceCSRF, map[string]any{"rating": 1, "text": "sabotaged"})
	resp.Body.Close()
	assert.NotEqual(t, http.StatusOK, resp.StatusCode,
		"Alice must NOT be able to edit Bob's review (review.Update checks UserID)")

	var rating int
	var text string
	db.QueryRow(`SELECT rating, text FROM reviews WHERE id=?`, bobReview).Scan(&rating, &text)
	assert.Equal(t, 5, rating, "Bob's review rating must not be overwritten")
	assert.Equal(t, "good service", text, "Bob's review text must not be overwritten")
}

// ─── Privacy center ──────────────────────────────────────────────────────────

func TestIDOR_PrivacyExport_ScopedToSessionUser(t *testing.T) {
	// The privacy handler does not expose an :id URL parameter — it always
	// reads the session user. This test guards against a regression where
	// someone adds an :id parameter without ownership scoping.
	srv, db := securityServer(t)
	_ = seedUser(t, db, "alice_exp", "regular_user")
	bob := seedUser(t, db, "bob_exp", "regular_user")

	// Seed a completed export for Bob.
	_, _ = db.Exec(`INSERT INTO data_export_requests (user_id, status, file_path, ready_at, expires_at)
		VALUES (?, 'ready', '/tmp/doesnotexist.zip', ?, ?)`,
		bob, time.Now(), time.Now().Add(24*time.Hour))

	aliceCli := newClient(t, srv)
	aliceCSRF := loginAs(t, aliceCli, srv.URL, "alice_exp")

	// Alice hits the status endpoint — should NOT see Bob's data.
	resp := doJSON(t, aliceCli, http.MethodGet,
		srv.URL+"/api/v1/users/me/export-request/status", aliceCSRF, nil)
	body := readBody(t, resp)
	// Either 404 (Alice has no export) or 200 with no mention of Bob's path.
	assert.NotContains(t, body, "doesnotexist.zip",
		"Alice's export status must never echo Bob's file path")
}

// ─── Mass assignment ─────────────────────────────────────────────────────────

func TestMassAssignment_TicketCreate_UserIDFromBodyIgnored(t *testing.T) {
	// A ticket create body that smuggles a user_id field must not steer the
	// server into recording the ticket against that id — the session user
	// is the only source of truth for ownership.
	srv, db := securityServer(t)
	alice := seedUser(t, db, "alice_mass", "regular_user")
	bob := seedUser(t, db, "bob_mass", "regular_user")

	// Seed an offering + address + category that Alice can legitimately use.
	_, offeringID, categoryID, addressID := seedTicketFor(t, db, alice)
	// Delete the seeded ticket so the count becomes a clean signal.
	_, _ = db.Exec(`DELETE FROM tickets WHERE user_id=?`, alice)

	aliceCli := newClient(t, srv)
	aliceCSRF := loginAs(t, aliceCli, srv.URL, "alice_mass")

	resp := doJSON(t, aliceCli, http.MethodPost, srv.URL+"/api/v1/tickets", aliceCSRF,
		map[string]any{
			// The server should parse only its declared fields.
			"user_id":         bob, // SHOULD BE IGNORED
			"offering_id":     offeringID,
			"category_id":     categoryID,
			"address_id":      addressID,
			"preferred_start": time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			"preferred_end":   time.Now().Add(26 * time.Hour).Format(time.RFC3339),
			"delivery_method": "pickup",
		})
	resp.Body.Close()
	// The create may 201 or 422 depending on validation; the critical property
	// is that if it succeeded, ownership landed on Alice (the session user).
	if resp.StatusCode == http.StatusCreated {
		var ownerID uint64
		err := db.QueryRow(`SELECT user_id FROM tickets WHERE offering_id=? ORDER BY id DESC LIMIT 1`,
			offeringID).Scan(&ownerID)
		require.NoError(t, err)
		assert.Equal(t, alice, ownerID,
			"ticket.user_id must be Alice (session user), NOT the body-smuggled id")
	}
	// Independently: Bob must have no tickets.
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM tickets WHERE user_id=?`, bob).Scan(&n)
	assert.Equal(t, 0, n, "Bob must not own a ticket as a side effect of Alice's request")
}
