package review_test

// Review submission rate limit: 10 per hour per authenticated user.
// The middleware is applied to create/update, so the 11th attempt must be
// rejected with 429 regardless of whether earlier attempts returned 201
// (first submission) or 409 (subsequent duplicates on the same ticket).

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTP_CreateReview_ThrottledAt10PerHour(t *testing.T) {
	srv, db := reviewServerWithDB(t)

	agentID := seedUser(t, db, "tra", "tra@t.l", "service_agent")
	userID := seedUser(t, db, "tru", "tru@t.l", "regular_user")
	_, ticketID := seedCompletedTicket(t, db, agentID, userID)

	client := srv.Client()
	client.Jar = newReviewJar()
	csrf := loginUser(t, client, srv.URL, "tru")

	url := srv.URL + "/api/v1/tickets/" + strconv.FormatUint(ticketID, 10) + "/reviews"

	// First 10 attempts are allowed by the limiter (the service layer will
	// succeed once with 201, and return 409 for the remaining 9 duplicates).
	var okSeen, dupSeen int
	for i := 0; i < 10; i++ {
		resp := doReviewJSON(t, client, http.MethodPost, url,
			map[string]interface{}{"rating": 4, "text": "attempt"}, csrf)
		require.NotEqual(t, http.StatusTooManyRequests, resp.StatusCode,
			"submission %d must NOT be rate-limited (limit is 10/hour)", i+1)
		switch resp.StatusCode {
		case http.StatusCreated:
			okSeen++
		case http.StatusConflict:
			dupSeen++
		}
		resp.Body.Close()
	}
	assert.Equal(t, 1, okSeen, "exactly one submission should succeed with 201 (duplicate)")
	assert.Equal(t, 9, dupSeen, "remaining 9 should be 409 duplicates")

	// 11th attempt must be 429 — rate limit exceeded.
	resp := doReviewJSON(t, client, http.MethodPost, url,
		map[string]interface{}{"rating": 4, "text": "eleventh"}, csrf)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode,
		"11th review submission within the hour must return 429")
}
