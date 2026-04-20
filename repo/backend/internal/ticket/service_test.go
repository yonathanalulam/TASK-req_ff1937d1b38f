package ticket_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eagle-point/service-portal/internal/models"
	"github.com/eagle-point/service-portal/internal/testutil"
	"github.com/eagle-point/service-portal/internal/ticket"
)

// ─── Unit: status transition matrix ──────────────────────────────────────────

func TestCheckTransition_Admin_AnyToClosed(t *testing.T) {
	// Admin can force any non-final ticket to Closed
	for _, from := range []string{"Accepted", "Dispatched", "In Service", "Completed"} {
		err := ticket.CheckTransition(from, "Closed", 99, []string{"administrator"}, 1)
		assert.NoError(t, err, "admin should close from %s", from)
	}
}

func TestCheckTransition_Admin_CannotTransitionFinalTicket(t *testing.T) {
	err := ticket.CheckTransition("Closed", "Completed", 99, []string{"administrator"}, 1)
	assert.ErrorIs(t, err, ticket.ErrInvalidTransition)

	err = ticket.CheckTransition("Cancelled", "Closed", 99, []string{"administrator"}, 1)
	assert.ErrorIs(t, err, ticket.ErrInvalidTransition)
}

func TestCheckTransition_Agent_ForwardOnly(t *testing.T) {
	cases := []struct {
		from, to string
		valid    bool
	}{
		{"Accepted", "Dispatched", true},
		{"Dispatched", "In Service", true},
		{"In Service", "Completed", true},
		{"Accepted", "In Service", false}, // skipping step
		{"Completed", "Closed", false},    // agent cannot close
		{"Dispatched", "Accepted", false}, // backwards
	}
	for _, tc := range cases {
		err := ticket.CheckTransition(tc.from, tc.to, 10, []string{"service_agent"}, 1)
		if tc.valid {
			assert.NoError(t, err, "agent %s → %s should be valid", tc.from, tc.to)
		} else {
			assert.ErrorIs(t, err, ticket.ErrInvalidTransition, "agent %s → %s should be invalid", tc.from, tc.to)
		}
	}
}

func TestCheckTransition_User_CancelBeforeDispatch(t *testing.T) {
	// Owner cancels from Accepted — allowed
	err := ticket.CheckTransition("Accepted", "Cancelled", 5, []string{"regular_user"}, 5)
	assert.NoError(t, err)

	// Owner cancels from Dispatched — not allowed
	err = ticket.CheckTransition("Dispatched", "Cancelled", 5, []string{"regular_user"}, 5)
	assert.ErrorIs(t, err, ticket.ErrInvalidTransition)

	// Non-owner regular user — forbidden
	err = ticket.CheckTransition("Accepted", "Cancelled", 99, []string{"regular_user"}, 5)
	assert.ErrorIs(t, err, ticket.ErrForbidden)
}

func TestCheckTransition_SameStatus_Rejected(t *testing.T) {
	err := ticket.CheckTransition("Accepted", "Accepted", 99, []string{"administrator"}, 1)
	assert.ErrorIs(t, err, ticket.ErrInvalidTransition)
}

func TestCheckTransition_UserOnlyCancels(t *testing.T) {
	// Owner tries to advance — not allowed for users
	err := ticket.CheckTransition("Accepted", "Dispatched", 5, []string{"regular_user"}, 5)
	assert.ErrorIs(t, err, ticket.ErrInvalidTransition)
}

// ─── Unit: CanView ───────────────────────────────────────────────────────────

func TestCanView_Owner(t *testing.T) {
	tk := &models.Ticket{UserID: 10}
	assert.True(t, ticket.CanView(10, []string{"regular_user"}, tk))
}

func TestCanView_NonOwner(t *testing.T) {
	tk := &models.Ticket{UserID: 10}
	assert.False(t, ticket.CanView(99, []string{"regular_user"}, tk))
}

func TestCanView_Administrator(t *testing.T) {
	tk := &models.Ticket{UserID: 10}
	assert.True(t, ticket.CanView(99, []string{"administrator"}, tk))
}

func TestCanView_ServiceAgent(t *testing.T) {
	tk := &models.Ticket{UserID: 10}
	assert.True(t, ticket.CanView(99, []string{"service_agent"}, tk))
}

// ─── Integration: ticket CRUD ────────────────────────────────────────────────

func TestTicket_Create_AndGet(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"ticket_attachments", "ticket_notes", "tickets",
		"service_offerings", "service_categories",
		"addresses",
		"login_attempts", "sessions", "user_roles", "users",
	)

	// Seed user, category, offering, address
	_, err := db.Exec(
		`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"ticketuser", "tu@test.local", "$2a$04$placeholder", "Ticket User",
	)
	require.NoError(t, err)
	var userID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='ticketuser'`).Scan(&userID)

	_, err = db.Exec(
		`INSERT INTO service_categories (name, slug, response_time_minutes) VALUES (?,?,?)`,
		"TicketCat", "ticket-cat", 60,
	)
	require.NoError(t, err)
	var catID uint64
	db.QueryRow(`SELECT id FROM service_categories WHERE slug='ticket-cat'`).Scan(&catID)

	_, err = db.Exec(
		`INSERT INTO service_offerings (agent_id, category_id, name, duration_minutes) VALUES (?,?,?,?)`,
		userID, catID, "Ticket Offering", 60,
	)
	require.NoError(t, err)
	var offeringID uint64
	db.QueryRow(`SELECT id FROM service_offerings WHERE name='Ticket Offering'`).Scan(&offeringID)

	_, err = db.Exec(
		`INSERT INTO addresses (user_id, label, address_line1_encrypted, city, state, zip, is_default)
		 VALUES (?, 'Home', ?, 'Portland', 'OR', '97201', 1)`,
		userID, []byte("100 Main St"),
	)
	require.NoError(t, err)
	var addrID uint64
	db.QueryRow(`SELECT id FROM addresses WHERE user_id=?`, userID).Scan(&addrID)

	svc := ticket.NewService(db, "storage/uploads")

	start := time.Now().Add(24 * time.Hour).UTC()
	end := start.Add(2 * time.Hour)

	tk, err := svc.Create(context.Background(), ticket.CreateInput{
		UserID:         userID,
		OfferingID:     offeringID,
		CategoryID:     catID,
		AddressID:      addrID,
		PreferredStart: start,
		PreferredEnd:   end,
		DeliveryMethod: "pickup",
		ShippingFee:    0,
	})
	require.NoError(t, err)
	assert.Equal(t, "Accepted", tk.Status)
	assert.NotNil(t, tk.SLADeadline, "SLA deadline must be set")
	assert.False(t, tk.SLABreached)

	// Get by ID
	got, err := svc.Get(context.Background(), tk.ID)
	require.NoError(t, err)
	assert.Equal(t, tk.ID, got.ID)
}

func TestTicket_Create_CategoryNotFound(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db, "ticket_attachments", "ticket_notes", "tickets")

	svc := ticket.NewService(db, "")
	_, err := svc.Create(context.Background(), ticket.CreateInput{
		UserID: 1, OfferingID: 1, CategoryID: 999999, AddressID: 1,
		PreferredStart: time.Now(), PreferredEnd: time.Now().Add(time.Hour),
	})
	assert.ErrorIs(t, err, ticket.ErrValidation)
}

func TestTicket_Create_InvalidPreferredWindow(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db, "ticket_attachments", "ticket_notes", "tickets")

	svc := ticket.NewService(db, "")
	now := time.Now()
	_, err := svc.Create(context.Background(), ticket.CreateInput{
		UserID: 1, OfferingID: 1, CategoryID: 1, AddressID: 1,
		PreferredStart: now.Add(time.Hour),
		PreferredEnd:   now, // end before start
	})
	assert.ErrorIs(t, err, ticket.ErrValidation)
}

// ─── Integration: SLA engine ─────────────────────────────────────────────────

func TestSLA_SweepBreachesOnce_FlipsExpired(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"ticket_attachments", "ticket_notes", "tickets",
		"service_offerings", "service_categories",
		"addresses",
		"login_attempts", "sessions", "user_roles", "users",
	)

	// Seed minimal FK dependencies
	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"slauser", "sla@test.local", "$2a$04$p", "SLA User")
	var userID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='slauser'`).Scan(&userID)

	db.Exec(`INSERT INTO service_categories (name, slug, response_time_minutes) VALUES (?,?,?)`, "SCat", "scat", 60)
	var catID uint64
	db.QueryRow(`SELECT id FROM service_categories WHERE slug='scat'`).Scan(&catID)

	db.Exec(`INSERT INTO service_offerings (agent_id, category_id, name, duration_minutes) VALUES (?,?,?,?)`,
		userID, catID, "SOff", 60)
	var offID uint64
	db.QueryRow(`SELECT id FROM service_offerings WHERE name='SOff'`).Scan(&offID)

	db.Exec(`INSERT INTO addresses (user_id, label, address_line1_encrypted, city, state, zip, is_default) VALUES (?,?,?,?,?,?,1)`,
		userID, "Home", []byte("1 Main"), "X", "NY", "10001")
	var addrID uint64
	db.QueryRow(`SELECT id FROM addresses WHERE user_id=?`, userID).Scan(&addrID)

	// Insert a ticket with a PAST sla_deadline
	past := time.Now().Add(-5 * time.Minute)
	db.Exec(
		`INSERT INTO tickets (user_id, offering_id, category_id, address_id, preferred_start, preferred_end,
		 delivery_method, status, sla_deadline)
		 VALUES (?,?,?,?,?,?, 'pickup', 'Accepted', ?)`,
		userID, offID, catID, addrID,
		time.Now().Add(time.Hour), time.Now().Add(2*time.Hour),
		past,
	)

	// Run the sweep
	require.NoError(t, ticket.SweepBreachesOnce(context.Background(), db))

	// Verify the breached flag is set
	var breached bool
	db.QueryRow(`SELECT sla_breached FROM tickets WHERE user_id=?`, userID).Scan(&breached)
	assert.True(t, breached, "sla_breached should be flipped to 1")
}

func TestSLA_SweepBreachesOnce_IgnoresCompletedTickets(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"ticket_attachments", "ticket_notes", "tickets",
		"service_offerings", "service_categories",
		"addresses",
		"login_attempts", "sessions", "user_roles", "users",
	)

	// Minimal FK seed
	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"slacomp", "slac@test.local", "$2a$04$p", "SC")
	var userID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='slacomp'`).Scan(&userID)
	db.Exec(`INSERT INTO service_categories (name, slug, response_time_minutes) VALUES (?,?,?)`, "SCCat", "sccat", 60)
	var catID uint64
	db.QueryRow(`SELECT id FROM service_categories WHERE slug='sccat'`).Scan(&catID)
	db.Exec(`INSERT INTO service_offerings (agent_id, category_id, name, duration_minutes) VALUES (?,?,?,?)`,
		userID, catID, "SCOff", 60)
	var offID uint64
	db.QueryRow(`SELECT id FROM service_offerings WHERE name='SCOff'`).Scan(&offID)
	db.Exec(`INSERT INTO addresses (user_id, label, address_line1_encrypted, city, state, zip, is_default) VALUES (?,?,?,?,?,?,1)`,
		userID, "Home", []byte("1 Main"), "X", "NY", "10001")
	var addrID uint64
	db.QueryRow(`SELECT id FROM addresses WHERE user_id=?`, userID).Scan(&addrID)

	past := time.Now().Add(-5 * time.Minute)
	db.Exec(
		`INSERT INTO tickets (user_id, offering_id, category_id, address_id, preferred_start, preferred_end,
		 delivery_method, status, sla_deadline)
		 VALUES (?,?,?,?,?,?, 'pickup', 'Completed', ?)`,
		userID, offID, catID, addrID,
		time.Now().Add(time.Hour), time.Now().Add(2*time.Hour), past,
	)

	require.NoError(t, ticket.SweepBreachesOnce(context.Background(), db))

	var breached bool
	db.QueryRow(`SELECT sla_breached FROM tickets WHERE user_id=?`, userID).Scan(&breached)
	assert.False(t, breached, "completed tickets must not be flagged as breached")
}

// ─── Integration: notes ──────────────────────────────────────────────────────

func TestTicket_CreateNote_AndList(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"ticket_attachments", "ticket_notes", "tickets",
		"service_offerings", "service_categories",
		"addresses",
		"login_attempts", "sessions", "user_roles", "users",
	)

	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"noteuser", "note@test.local", "$2a$04$p", "Note")
	var userID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='noteuser'`).Scan(&userID)
	db.Exec(`INSERT INTO service_categories (name, slug, response_time_minutes) VALUES (?,?,?)`, "NCat", "ncat", 60)
	var catID uint64
	db.QueryRow(`SELECT id FROM service_categories WHERE slug='ncat'`).Scan(&catID)
	db.Exec(`INSERT INTO service_offerings (agent_id, category_id, name, duration_minutes) VALUES (?,?,?,?)`,
		userID, catID, "NOff", 60)
	var offID uint64
	db.QueryRow(`SELECT id FROM service_offerings WHERE name='NOff'`).Scan(&offID)
	db.Exec(`INSERT INTO addresses (user_id, label, address_line1_encrypted, city, state, zip, is_default) VALUES (?,?,?,?,?,?,1)`,
		userID, "Home", []byte("1 Main"), "X", "NY", "10001")
	var addrID uint64
	db.QueryRow(`SELECT id FROM addresses WHERE user_id=?`, userID).Scan(&addrID)

	svc := ticket.NewService(db, "")
	tk, err := svc.Create(context.Background(), ticket.CreateInput{
		UserID: userID, OfferingID: offID, CategoryID: catID, AddressID: addrID,
		PreferredStart: time.Now().Add(time.Hour),
		PreferredEnd:   time.Now().Add(2 * time.Hour),
	})
	require.NoError(t, err)

	n, err := svc.CreateNote(context.Background(), tk.ID, userID, "First note")
	require.NoError(t, err)
	assert.NotZero(t, n.ID)
	assert.Equal(t, "First note", n.Content)

	notes, err := svc.ListNotes(context.Background(), tk.ID)
	require.NoError(t, err)
	assert.Len(t, notes, 1)
}

func TestTicket_CreateNote_EmptyContent_Fails(t *testing.T) {
	db := testutil.DBOrSkip(t)
	svc := ticket.NewService(db, "")
	_, err := svc.CreateNote(context.Background(), 1, 1, "   ")
	assert.ErrorIs(t, err, ticket.ErrValidation)
}

// ─── Integration: list filtering by role ─────────────────────────────────────

func TestTicket_List_ScopesByRole(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"ticket_attachments", "ticket_notes", "tickets",
		"service_offerings", "service_categories",
		"addresses",
		"login_attempts", "sessions", "user_roles", "users",
	)

	// Two users
	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?),(?,?,?,?)`,
		"ua", "ua@t.l", "$2a$04$p", "A",
		"ub", "ub@t.l", "$2a$04$p", "B")
	var uaID, ubID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='ua'`).Scan(&uaID)
	db.QueryRow(`SELECT id FROM users WHERE username='ub'`).Scan(&ubID)

	db.Exec(`INSERT INTO service_categories (name, slug, response_time_minutes) VALUES (?,?,?)`, "LC", "lc", 60)
	var catID uint64
	db.QueryRow(`SELECT id FROM service_categories WHERE slug='lc'`).Scan(&catID)
	db.Exec(`INSERT INTO service_offerings (agent_id, category_id, name, duration_minutes) VALUES (?,?,?,?)`,
		uaID, catID, "LO", 60)
	var offID uint64
	db.QueryRow(`SELECT id FROM service_offerings WHERE name='LO'`).Scan(&offID)
	db.Exec(`INSERT INTO addresses (user_id, label, address_line1_encrypted, city, state, zip, is_default) VALUES (?,?,?,?,?,?,1)`,
		uaID, "H", []byte("1 Main"), "X", "NY", "10001")
	var addrA uint64
	db.QueryRow(`SELECT id FROM addresses WHERE user_id=?`, uaID).Scan(&addrA)
	db.Exec(`INSERT INTO addresses (user_id, label, address_line1_encrypted, city, state, zip, is_default) VALUES (?,?,?,?,?,?,1)`,
		ubID, "H", []byte("1 Main"), "X", "NY", "10001")
	var addrB uint64
	db.QueryRow(`SELECT id FROM addresses WHERE user_id=?`, ubID).Scan(&addrB)

	svc := ticket.NewService(db, "")
	ctx := context.Background()

	_, err := svc.Create(ctx, ticket.CreateInput{
		UserID: uaID, OfferingID: offID, CategoryID: catID, AddressID: addrA,
		PreferredStart: time.Now().Add(time.Hour), PreferredEnd: time.Now().Add(2 * time.Hour),
	})
	require.NoError(t, err)
	_, err = svc.Create(ctx, ticket.CreateInput{
		UserID: ubID, OfferingID: offID, CategoryID: catID, AddressID: addrB,
		PreferredStart: time.Now().Add(time.Hour), PreferredEnd: time.Now().Add(2 * time.Hour),
	})
	require.NoError(t, err)

	// Regular user sees only their own
	list, err := svc.List(ctx, uaID, []string{"regular_user"}, ticket.ListFilter{})
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, uaID, list[0].UserID)

	// Admin sees all
	list, err = svc.List(ctx, 999, []string{"administrator"}, ticket.ListFilter{})
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

// ─── Lifecycle: upcoming_end reminder dispatch ───────────────────────────────

// TestUpdateStatus_InService_DispatchesUpcomingEnd asserts the ticket service
// fires an upcoming_end notification (in addition to the generic status-change
// one) when a ticket enters In Service. The end-time reminder requirement is
// the counterpart to upcoming_start and must be wired in the lifecycle flow.
func TestUpdateStatus_InService_DispatchesUpcomingEnd(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"ticket_attachments", "ticket_notes", "tickets",
		"service_offerings", "service_categories",
		"addresses",
		"login_attempts", "sessions", "user_roles", "users",
	)

	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?),(?,?,?,?)`,
		"owner", "o@t.l", "$2a$04$p", "O",
		"agent", "a@t.l", "$2a$04$p", "A",
	)
	var ownerID, agentID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='owner'`).Scan(&ownerID)
	db.QueryRow(`SELECT id FROM users WHERE username='agent'`).Scan(&agentID)

	db.Exec(`INSERT INTO service_categories (name, slug, response_time_minutes) VALUES (?,?,?)`, "UEC", "uec", 60)
	var catID uint64
	db.QueryRow(`SELECT id FROM service_categories WHERE slug='uec'`).Scan(&catID)
	db.Exec(`INSERT INTO service_offerings (agent_id, category_id, name, duration_minutes) VALUES (?,?,?,?)`,
		agentID, catID, "UEOff", 60)
	var offID uint64
	db.QueryRow(`SELECT id FROM service_offerings WHERE name='UEOff'`).Scan(&offID)
	db.Exec(`INSERT INTO addresses (user_id, label, address_line1_encrypted, city, state, zip, is_default) VALUES (?,?,?,?,?,?,1)`,
		ownerID, "H", []byte("1 Main"), "X", "NY", "10001")
	var addrID uint64
	db.QueryRow(`SELECT id FROM addresses WHERE user_id=?`, ownerID).Scan(&addrID)

	// Insert a Dispatched ticket so the agent can move it to In Service.
	start := time.Now().Add(time.Hour)
	end := start.Add(2 * time.Hour)
	db.Exec(
		`INSERT INTO tickets (user_id, offering_id, category_id, address_id, preferred_start, preferred_end,
		 delivery_method, status, assigned_agent_id)
		 VALUES (?,?,?,?,?,?, 'pickup', 'Dispatched', ?)`,
		ownerID, offID, catID, addrID, start, end, agentID,
	)
	var tkID uint64
	db.QueryRow(`SELECT id FROM tickets WHERE user_id=?`, ownerID).Scan(&tkID)

	svc := ticket.NewService(db, "")

	var dispatched []string
	svc.SetNotifier(func(ctx context.Context, userID uint64, code string, vars map[string]any) error {
		dispatched = append(dispatched, code)
		return nil
	})

	_, err := svc.UpdateStatus(context.Background(), tkID, agentID,
		[]string{"service_agent"}, models.TicketStatusInService, "")
	require.NoError(t, err)

	assert.Contains(t, dispatched, models.NotifTicketStatusChange)
	assert.Contains(t, dispatched, models.NotifUpcomingEnd,
		"upcoming_end reminder must fire when ticket enters In Service")
}
