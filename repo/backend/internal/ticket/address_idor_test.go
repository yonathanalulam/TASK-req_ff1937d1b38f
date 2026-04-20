package ticket_test

// Object-level authorization test: ticket creation must reject a request that
// references an address_id owned by a different user. Without this check, an
// authenticated user could bind their ticket to another user's address by
// simply guessing the id (IDOR).

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTP_CreateTicket_RejectsForeignAddress(t *testing.T) {
	srv, db := ticketServerWithDB(t)

	agentID := seedUserWithRole(t, db, "agentidor", "agentidor@t.l", "service_agent")

	// Victim owns the address; attacker attempts to use it.
	victimID := seedUserWithRole(t, db, "victim", "victim@t.l", "regular_user")
	attackerID := seedUserWithRole(t, db, "attacker", "attacker@t.l", "regular_user")
	_ = attackerID

	catID, offID, victimAddrID := seedTicketDependencies(t, db, agentID, victimID)

	client := srv.Client()
	client.Jar = newTicketJar()
	csrf := loginSeed(t, client, srv.URL, "attacker")

	resp := doJSONt(t, client, http.MethodPost, srv.URL+"/api/v1/tickets",
		map[string]interface{}{
			"offering_id":     offID,
			"category_id":     catID,
			"address_id":      victimAddrID, // ← not owned by attacker
			"preferred_start": "2026-06-01T10:00:00Z",
			"preferred_end":   "2026-06-01T12:00:00Z",
			"delivery_method": "pickup",
		}, csrf)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode,
		"foreign address_id on ticket create must be rejected with 403")

	// Sanity: no ticket row was inserted for the attacker.
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM tickets WHERE user_id = ?`, attackerID).Scan(&n))
	assert.Equal(t, 0, n, "no ticket row should have been created")

	// Drain response
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
}

func TestHTTP_CreateTicket_RejectsUnknownAddress(t *testing.T) {
	srv, db := ticketServerWithDB(t)
	agentID := seedUserWithRole(t, db, "agentidor2", "agentidor2@t.l", "service_agent")
	userID := seedUserWithRole(t, db, "user2", "user2@t.l", "regular_user")
	catID, offID, _ := seedTicketDependencies(t, db, agentID, userID)

	client := srv.Client()
	client.Jar = newTicketJar()
	csrf := loginSeed(t, client, srv.URL, "user2")

	resp := doJSONt(t, client, http.MethodPost, srv.URL+"/api/v1/tickets",
		map[string]interface{}{
			"offering_id":     offID,
			"category_id":     catID,
			"address_id":      9_999_999, // nonexistent
			"preferred_start": "2026-06-01T10:00:00Z",
			"preferred_end":   "2026-06-01T12:00:00Z",
			"delivery_method": "pickup",
		}, csrf)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode,
		"unknown address_id must be rejected as validation error")
}
