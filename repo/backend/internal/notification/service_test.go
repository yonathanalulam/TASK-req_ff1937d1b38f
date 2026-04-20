package notification_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eagle-point/service-portal/internal/models"
	"github.com/eagle-point/service-portal/internal/notification"
	"github.com/eagle-point/service-portal/internal/testutil"
)

// ─── Unit: template rendering ────────────────────────────────────────────────

func TestRender_SubstitutesVariables(t *testing.T) {
	tmpl := &models.NotificationTemplate{
		Code:          "x",
		TitleTemplate: "Ticket #{{.TicketID}} — {{.Status}}",
		BodyTemplate:  "Your ticket {{.TicketID}} moved to {{.Status}}.",
	}
	title, body, err := notification.Render(tmpl, map[string]any{
		"TicketID": 42, "Status": "Dispatched",
	})
	require.NoError(t, err)
	assert.Equal(t, "Ticket #42 — Dispatched", title)
	assert.Equal(t, "Your ticket 42 moved to Dispatched.", body)
}

func TestRender_MissingVariable_RendersZeroValue(t *testing.T) {
	tmpl := &models.NotificationTemplate{
		TitleTemplate: "{{.Foo}}",
		BodyTemplate:  "{{.Bar}}",
	}
	title, body, err := notification.Render(tmpl, map[string]any{})
	require.NoError(t, err)
	// text/template renders missing keys as "<no value>" by default
	assert.Contains(t, title, "no value")
	assert.Contains(t, body, "no value")
}

func TestRender_ParseError(t *testing.T) {
	tmpl := &models.NotificationTemplate{
		TitleTemplate: "{{.Foo", // unclosed
		BodyTemplate:  "ok",
	}
	_, _, err := notification.Render(tmpl, nil)
	assert.Error(t, err)
}

// ─── Integration: dispatch + retrieval ───────────────────────────────────────

func setupNotifTables(t *testing.T) {
	t.Helper()
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"notification_outbox", "notifications", "notification_templates",
		"user_preferences", "user_roles", "users",
	)
}

func TestDispatch_InsertsRow(t *testing.T) {
	db := testutil.DBOrSkip(t)
	setupNotifTables(t)

	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"nuser", "n@t.l", "$2a$04$p", "N")
	var userID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='nuser'`).Scan(&userID)
	db.Exec(`INSERT INTO user_preferences (user_id, notify_in_app) VALUES (?, 1)`, userID)

	svc := notification.NewService(db)
	_, err := svc.UpsertTemplate(context.Background(), "test_event",
		"Hello {{.Name}}", "Body {{.Name}}")
	require.NoError(t, err)

	n, err := svc.Dispatch(context.Background(), userID, "test_event", map[string]any{"Name": "Alice"})
	require.NoError(t, err)
	assert.Equal(t, "Hello Alice", n.Title)
	assert.Equal(t, "Body Alice", n.Body)
	assert.False(t, n.IsRead)
}

func TestDispatch_RoutesToOutbox_WhenInAppDisabled(t *testing.T) {
	db := testutil.DBOrSkip(t)
	setupNotifTables(t)

	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"obox", "o@t.l", "$2a$04$p", "O")
	var userID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='obox'`).Scan(&userID)
	// notify_in_app = false → outbox routing
	db.Exec(`INSERT INTO user_preferences (user_id, notify_in_app) VALUES (?, 0)`, userID)

	svc := notification.NewService(db)
	svc.UpsertTemplate(context.Background(), "ev", "T", "B")

	n, err := svc.Dispatch(context.Background(), userID, "ev", nil)
	require.NoError(t, err)

	var count int
	db.QueryRow(`SELECT COUNT(*) FROM notification_outbox WHERE notification_id=?`, n.ID).Scan(&count)
	assert.Equal(t, 1, count, "outbox entry expected when notify_in_app=false")
}

func TestDispatch_NotificationStillCreated_EvenWhenOutboxRouted(t *testing.T) {
	// The notification row must exist regardless — outbox is just a delivery hint.
	db := testutil.DBOrSkip(t)
	setupNotifTables(t)

	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"both", "b@t.l", "$2a$04$p", "B")
	var userID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='both'`).Scan(&userID)
	db.Exec(`INSERT INTO user_preferences (user_id, notify_in_app) VALUES (?, 0)`, userID)

	svc := notification.NewService(db)
	svc.UpsertTemplate(context.Background(), "ev2", "T", "B")
	_, err := svc.Dispatch(context.Background(), userID, "ev2", nil)
	require.NoError(t, err)

	var n int
	db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE user_id=?`, userID).Scan(&n)
	assert.Equal(t, 1, n)
}

func TestList_AndUnreadCount_AndMarkRead(t *testing.T) {
	db := testutil.DBOrSkip(t)
	setupNotifTables(t)

	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"reader", "r@t.l", "$2a$04$p", "R")
	var userID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='reader'`).Scan(&userID)
	db.Exec(`INSERT INTO user_preferences (user_id, notify_in_app) VALUES (?, 1)`, userID)

	svc := notification.NewService(db)
	svc.UpsertTemplate(context.Background(), "x", "Title", "Body")

	for i := 0; i < 3; i++ {
		svc.Dispatch(context.Background(), userID, "x", nil)
	}

	page, err := svc.List(context.Background(), userID, "", 0, 10)
	require.NoError(t, err)
	assert.Len(t, page.Items, 3)

	count, err := svc.UnreadCount(context.Background(), userID)
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	// Mark one read
	require.NoError(t, svc.MarkRead(context.Background(), page.Items[0].ID, userID))
	count, _ = svc.UnreadCount(context.Background(), userID)
	assert.Equal(t, 2, count)

	// Mark all
	n, err := svc.MarkAllRead(context.Background(), userID)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)

	count, _ = svc.UnreadCount(context.Background(), userID)
	assert.Equal(t, 0, count)
}

func TestList_FilterByReadState(t *testing.T) {
	db := testutil.DBOrSkip(t)
	setupNotifTables(t)

	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"flt", "f@t.l", "$2a$04$p", "F")
	var userID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='flt'`).Scan(&userID)
	db.Exec(`INSERT INTO user_preferences (user_id, notify_in_app) VALUES (?, 1)`, userID)

	svc := notification.NewService(db)
	svc.UpsertTemplate(context.Background(), "x", "T", "B")
	for i := 0; i < 2; i++ {
		svc.Dispatch(context.Background(), userID, "x", nil)
	}
	page, _ := svc.List(context.Background(), userID, "", 0, 10)
	require.NoError(t, svc.MarkRead(context.Background(), page.Items[0].ID, userID))

	unread, err := svc.List(context.Background(), userID, "unread", 0, 10)
	require.NoError(t, err)
	assert.Len(t, unread.Items, 1)
	assert.False(t, unread.Items[0].IsRead)

	read, err := svc.List(context.Background(), userID, "read", 0, 10)
	require.NoError(t, err)
	assert.Len(t, read.Items, 1)
	assert.True(t, read.Items[0].IsRead)
}

func TestList_CursorPagination(t *testing.T) {
	db := testutil.DBOrSkip(t)
	setupNotifTables(t)

	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"pag", "p@t.l", "$2a$04$p", "P")
	var userID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='pag'`).Scan(&userID)
	db.Exec(`INSERT INTO user_preferences (user_id, notify_in_app) VALUES (?, 1)`, userID)

	svc := notification.NewService(db)
	svc.UpsertTemplate(context.Background(), "x", "T", "B")
	for i := 0; i < 5; i++ {
		svc.Dispatch(context.Background(), userID, "x", nil)
	}

	page1, err := svc.List(context.Background(), userID, "", 0, 3)
	require.NoError(t, err)
	assert.Len(t, page1.Items, 3)
	assert.NotZero(t, page1.NextCursor)

	page2, err := svc.List(context.Background(), userID, "", page1.NextCursor, 3)
	require.NoError(t, err)
	assert.Len(t, page2.Items, 2)
	assert.Zero(t, page2.NextCursor)
}

func TestSeedDefaultTemplates_Idempotent(t *testing.T) {
	db := testutil.DBOrSkip(t)
	setupNotifTables(t)

	require.NoError(t, notification.SeedDefaultTemplates(context.Background(), db))
	// Run twice — INSERT IGNORE should not error or duplicate
	require.NoError(t, notification.SeedDefaultTemplates(context.Background(), db))

	svc := notification.NewService(db)
	templates, err := svc.ListTemplates(context.Background())
	require.NoError(t, err)
	// Each defined code must be present exactly once
	codes := make(map[string]int)
	for _, tmpl := range templates {
		codes[tmpl.Code]++
	}
	assert.Equal(t, 1, codes[models.NotifTicketStatusChange])
	assert.Equal(t, 1, codes[models.NotifSLABreach])
	assert.Equal(t, 1, codes[models.NotifAccountLockout])
	// Upcoming start AND upcoming end templates must both be seeded — the
	// notification requirement covers both lifecycle boundaries.
	assert.Equal(t, 1, codes[models.NotifUpcomingStart])
	assert.Equal(t, 1, codes[models.NotifUpcomingEnd])
}

// TestDispatch_UpcomingEnd_RendersEndTime confirms the upcoming_end template
// renders with the End variable, so ticket.UpdateStatus's InService hook
// produces a readable notification body rather than "<no value>".
func TestDispatch_UpcomingEnd_RendersEndTime(t *testing.T) {
	db := testutil.DBOrSkip(t)
	setupNotifTables(t)

	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"endu", "e@t.l", "$2a$04$p", "E")
	var userID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='endu'`).Scan(&userID)
	db.Exec(`INSERT INTO user_preferences (user_id, notify_in_app) VALUES (?, 1)`, userID)

	require.NoError(t, notification.SeedDefaultTemplates(context.Background(), db))

	svc := notification.NewService(db)
	n, err := svc.Dispatch(context.Background(), userID, models.NotifUpcomingEnd, map[string]any{
		"TicketID": 42,
		"End":      "2026-04-21T10:00:00Z",
	})
	require.NoError(t, err)
	assert.Equal(t, models.NotifUpcomingEnd, n.TemplateCode)
	assert.Contains(t, n.Title, "42")
	assert.Contains(t, n.Body, "2026-04-21T10:00:00Z")
}

func TestListOutbox_ReturnsJoinedNotification(t *testing.T) {
	db := testutil.DBOrSkip(t)
	setupNotifTables(t)

	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"jox", "j@t.l", "$2a$04$p", "J")
	var userID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='jox'`).Scan(&userID)
	db.Exec(`INSERT INTO user_preferences (user_id, notify_in_app) VALUES (?, 0)`, userID)

	svc := notification.NewService(db)
	svc.UpsertTemplate(context.Background(), "ox", "Hello", "World")
	svc.Dispatch(context.Background(), userID, "ox", nil)

	out, err := svc.ListOutbox(context.Background(), userID, 10)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "Hello", out[0].Notification.Title)
	assert.Equal(t, "World", out[0].Notification.Body)
}
