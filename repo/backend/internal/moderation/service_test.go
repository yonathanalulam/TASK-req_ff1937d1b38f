package moderation_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eagle-point/service-portal/internal/models"
	"github.com/eagle-point/service-portal/internal/moderation"
	"github.com/eagle-point/service-portal/internal/testutil"
)

// ─── Unit: ScanText ──────────────────────────────────────────────────────────

func TestScanText_NoMatch(t *testing.T) {
	dict := map[string]string{"banned": "prohibited", "warn": "borderline"}
	r := moderation.ScanText("hello world", dict)
	assert.Equal(t, "", r.Class)
	assert.Empty(t, r.FlaggedTerms)
}

func TestScanText_ProhibitedBeatsBorderline(t *testing.T) {
	dict := map[string]string{"banned": "prohibited", "warn": "borderline"}
	r := moderation.ScanText("warn this banned text", dict)
	assert.Equal(t, "prohibited", r.Class)
	assert.True(t, r.HasProhibited())
	assert.Contains(t, r.FlaggedTerms, "banned")
	assert.Contains(t, r.FlaggedTerms, "warn")
}

func TestScanText_OnlyBorderline(t *testing.T) {
	dict := map[string]string{"warn": "borderline", "watch": "borderline"}
	r := moderation.ScanText("WARN: please WATCH", dict)
	assert.Equal(t, "borderline", r.Class)
	assert.True(t, r.HasBorderline())
	assert.Len(t, r.FlaggedTerms, 2)
}

func TestScanText_CaseInsensitive(t *testing.T) {
	dict := map[string]string{"banned": "prohibited"}
	r := moderation.ScanText("BANNED!", dict)
	assert.Equal(t, "prohibited", r.Class)
}

func TestScanText_NoPartialWordMatch(t *testing.T) {
	// "ban" should not match the substring inside "banner" or "banal"
	dict := map[string]string{"ban": "prohibited"}
	r := moderation.ScanText("the banner is banal", dict)
	assert.Equal(t, "", r.Class, "partial-word matches must not trigger")
}

func TestScanText_PunctuationDelimits(t *testing.T) {
	dict := map[string]string{"foo": "prohibited"}
	r := moderation.ScanText("foo, bar, baz!", dict)
	assert.Equal(t, "prohibited", r.Class)
	assert.Equal(t, []string{"foo"}, r.FlaggedTerms)
}

func TestScanText_DeduplicatesFlagged(t *testing.T) {
	dict := map[string]string{"foo": "borderline"}
	r := moderation.ScanText("foo foo foo", dict)
	assert.Len(t, r.FlaggedTerms, 1)
}

func TestScanText_EmptyDict(t *testing.T) {
	r := moderation.ScanText("anything goes", nil)
	assert.Equal(t, "", r.Class)
}

// ─── Integration: term CRUD + cache ──────────────────────────────────────────

func setupModTables(t *testing.T) {
	t.Helper()
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"violation_records", "moderation_actions", "moderation_queue",
		"sensitive_terms",
		"qa_posts", "qa_threads",
		"review_reports", "review_images", "reviews",
		"ticket_attachments", "ticket_notes", "tickets",
		"service_offerings", "service_categories",
		"addresses", "login_attempts", "sessions", "user_roles", "users",
	)
}

func TestAddTerm_RefreshesCache(t *testing.T) {
	db := testutil.DBOrSkip(t)
	setupModTables(t)

	svc := moderation.NewService(db)
	// Empty dictionary → no match
	r := svc.Screen(context.Background(), "first banned word")
	assert.Equal(t, "", r.Class)

	_, err := svc.AddTerm(context.Background(), "banned", "prohibited")
	require.NoError(t, err)

	// Cache should reflect the new term immediately
	r = svc.Screen(context.Background(), "first banned word")
	assert.Equal(t, "prohibited", r.Class)
}

func TestAddTerm_DuplicateRejected(t *testing.T) {
	db := testutil.DBOrSkip(t)
	setupModTables(t)

	svc := moderation.NewService(db)
	_, err := svc.AddTerm(context.Background(), "spam", "prohibited")
	require.NoError(t, err)
	_, err = svc.AddTerm(context.Background(), "spam", "borderline")
	assert.ErrorIs(t, err, moderation.ErrDuplicate)
}

func TestAddTerm_InvalidClass(t *testing.T) {
	db := testutil.DBOrSkip(t)
	svc := moderation.NewService(db)
	_, err := svc.AddTerm(context.Background(), "x", "weird")
	assert.ErrorIs(t, err, moderation.ErrValidation)
}

func TestDeleteTerm_RefreshesCache(t *testing.T) {
	db := testutil.DBOrSkip(t)
	setupModTables(t)

	svc := moderation.NewService(db)
	added, err := svc.AddTerm(context.Background(), "removeme", "prohibited")
	require.NoError(t, err)

	r := svc.Screen(context.Background(), "removeme")
	assert.Equal(t, "prohibited", r.Class)

	require.NoError(t, svc.DeleteTerm(context.Background(), added.ID))

	r = svc.Screen(context.Background(), "removeme")
	assert.Equal(t, "", r.Class, "deleted term should no longer match")
}

// ─── Integration: queue approve / reject + freeze escalation ────────────────

func TestRejectItem_FirstViolation_AppliesShortFreeze(t *testing.T) {
	db := testutil.DBOrSkip(t)
	setupModTables(t)

	// Seed a user, ticket, review (the content to reject)
	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?),(?,?,?,?)`,
		"author1", "a1@t.l", "$2a$04$p", "A1",
		"mod1", "m1@t.l", "$2a$04$p", "M1")
	var authorID, modID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='author1'`).Scan(&authorID)
	db.QueryRow(`SELECT id FROM users WHERE username='mod1'`).Scan(&modID)

	db.Exec(`INSERT INTO service_categories (name, slug, response_time_minutes) VALUES (?,?,?)`, "MC1", "mc1", 60)
	var catID uint64
	db.QueryRow(`SELECT id FROM service_categories WHERE slug='mc1'`).Scan(&catID)
	db.Exec(`INSERT INTO service_offerings (agent_id, category_id, name, duration_minutes) VALUES (?,?,?,?)`,
		authorID, catID, "MO1", 60)
	var offID uint64
	db.QueryRow(`SELECT id FROM service_offerings WHERE name='MO1'`).Scan(&offID)
	db.Exec(`INSERT INTO addresses (user_id, label, address_line1_encrypted, city, state, zip, is_default) VALUES (?,?,?,?,?,?,1)`,
		authorID, "H", []byte("1 Main"), "X", "NY", "10001")
	var addrID uint64
	db.QueryRow(`SELECT id FROM addresses WHERE user_id=?`, authorID).Scan(&addrID)
	db.Exec(`INSERT INTO tickets (user_id, offering_id, category_id, address_id, preferred_start, preferred_end,
		delivery_method, status) VALUES (?,?,?,?,'2026-06-01 10:00:00','2026-06-01 12:00:00','pickup','Completed')`,
		authorID, offID, catID, addrID)
	var ticketID uint64
	db.QueryRow(`SELECT id FROM tickets WHERE user_id=?`, authorID).Scan(&ticketID)
	db.Exec(`INSERT INTO reviews (ticket_id, user_id, offering_id, rating, text, status)
		VALUES (?,?,?,?,?, 'pending_moderation')`,
		ticketID, authorID, offID, 1, "borderline content here")
	var reviewID uint64
	db.QueryRow(`SELECT id FROM reviews WHERE ticket_id=?`, ticketID).Scan(&reviewID)

	svc := moderation.NewService(db)
	item, err := svc.EnqueueBorderline(context.Background(), models.ModContentReview, reviewID, "borderline content here", []string{"borderline"})
	require.NoError(t, err)

	_, freezeUntil, err := svc.RejectItem(context.Background(), item.ID, modID, "abusive language")
	require.NoError(t, err)
	require.NotNil(t, freezeUntil, "first rejection should freeze the user")

	// Verify freeze is around 24 hours out (allow 5 minutes of slack)
	expected := time.Now().Add(24 * time.Hour)
	delta := freezeUntil.Sub(expected)
	assert.True(t, delta < 5*time.Minute && delta > -5*time.Minute,
		"first freeze should be ~24h, got delta %v", delta)

	// Verify violation_records row exists
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM violation_records WHERE user_id=?`, authorID).Scan(&n)
	assert.Equal(t, 1, n)

	// Verify users.posting_freeze_until is set
	until, err := svc.IsUserFrozen(context.Background(), authorID)
	require.NoError(t, err)
	assert.NotNil(t, until)
}

func TestRejectItem_SecondViolation_AppliesLongFreeze(t *testing.T) {
	db := testutil.DBOrSkip(t)
	setupModTables(t)

	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?),(?,?,?,?)`,
		"author2", "a2@t.l", "$2a$04$p", "A2",
		"mod2", "m2@t.l", "$2a$04$p", "M2")
	var authorID, modID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='author2'`).Scan(&authorID)
	db.QueryRow(`SELECT id FROM users WHERE username='mod2'`).Scan(&modID)

	// Seed two completed tickets + reviews (one per offence)
	db.Exec(`INSERT INTO service_categories (name, slug, response_time_minutes) VALUES (?,?,?)`, "MC2", "mc2", 60)
	var catID uint64
	db.QueryRow(`SELECT id FROM service_categories WHERE slug='mc2'`).Scan(&catID)
	db.Exec(`INSERT INTO service_offerings (agent_id, category_id, name, duration_minutes) VALUES (?,?,?,?)`,
		authorID, catID, "MO2", 60)
	var offID uint64
	db.QueryRow(`SELECT id FROM service_offerings WHERE name='MO2'`).Scan(&offID)
	db.Exec(`INSERT INTO addresses (user_id, label, address_line1_encrypted, city, state, zip, is_default) VALUES (?,?,?,?,?,?,1)`,
		authorID, "H", []byte("1 Main"), "X", "NY", "10001")
	var addrID uint64
	db.QueryRow(`SELECT id FROM addresses WHERE user_id=?`, authorID).Scan(&addrID)

	var reviewIDs [2]uint64
	for i := 0; i < 2; i++ {
		db.Exec(`INSERT INTO tickets (user_id, offering_id, category_id, address_id, preferred_start, preferred_end,
			delivery_method, status) VALUES (?,?,?,?,'2026-06-01 10:00:00','2026-06-01 12:00:00','pickup','Completed')`,
			authorID, offID, catID, addrID)
	}
	rows, _ := db.Query(`SELECT id FROM tickets WHERE user_id=? ORDER BY id ASC`, authorID)
	var ticketIDs []uint64
	for rows.Next() {
		var id uint64
		rows.Scan(&id)
		ticketIDs = append(ticketIDs, id)
	}
	rows.Close()

	for i, tid := range ticketIDs {
		db.Exec(`INSERT INTO reviews (ticket_id, user_id, offering_id, rating, text, status)
			VALUES (?,?,?,?,?, 'pending_moderation')`,
			tid, authorID, offID, 1, "warn warn")
		var rid uint64
		db.QueryRow(`SELECT id FROM reviews WHERE ticket_id=?`, tid).Scan(&rid)
		reviewIDs[i] = rid
	}

	svc := moderation.NewService(db)

	// First rejection
	item1, err := svc.EnqueueBorderline(context.Background(), models.ModContentReview, reviewIDs[0], "warn warn", []string{"warn"})
	require.NoError(t, err)
	_, _, err = svc.RejectItem(context.Background(), item1.ID, modID, "first")
	require.NoError(t, err)

	// Second rejection — should escalate to 7-day freeze
	item2, err := svc.EnqueueBorderline(context.Background(), models.ModContentReview, reviewIDs[1], "warn warn", []string{"warn"})
	require.NoError(t, err)
	_, freezeUntil, err := svc.RejectItem(context.Background(), item2.ID, modID, "second")
	require.NoError(t, err)
	require.NotNil(t, freezeUntil)

	expected := time.Now().Add(7 * 24 * time.Hour)
	delta := freezeUntil.Sub(expected)
	assert.True(t, delta < 5*time.Minute && delta > -5*time.Minute,
		"second freeze should be ~7d, got delta %v", delta)

	// Two violation records expected
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM violation_records WHERE user_id=?`, authorID).Scan(&n)
	assert.Equal(t, 2, n)
}

func TestApproveItem_PromotesContent(t *testing.T) {
	db := testutil.DBOrSkip(t)
	setupModTables(t)

	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?),(?,?,?,?)`,
		"approver", "ap@t.l", "$2a$04$p", "AP",
		"approwner", "aw@t.l", "$2a$04$p", "AW")
	var modID, authorID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='approver'`).Scan(&modID)
	db.QueryRow(`SELECT id FROM users WHERE username='approwner'`).Scan(&authorID)

	db.Exec(`INSERT INTO service_categories (name, slug, response_time_minutes) VALUES (?,?,?)`, "AC", "ac", 60)
	var catID uint64
	db.QueryRow(`SELECT id FROM service_categories WHERE slug='ac'`).Scan(&catID)
	db.Exec(`INSERT INTO service_offerings (agent_id, category_id, name, duration_minutes) VALUES (?,?,?,?)`,
		authorID, catID, "AO", 60)
	var offID uint64
	db.QueryRow(`SELECT id FROM service_offerings WHERE name='AO'`).Scan(&offID)
	db.Exec(`INSERT INTO addresses (user_id, label, address_line1_encrypted, city, state, zip, is_default) VALUES (?,?,?,?,?,?,1)`,
		authorID, "H", []byte("1 Main"), "X", "NY", "10001")
	var addrID uint64
	db.QueryRow(`SELECT id FROM addresses WHERE user_id=?`, authorID).Scan(&addrID)
	db.Exec(`INSERT INTO tickets (user_id, offering_id, category_id, address_id, preferred_start, preferred_end,
		delivery_method, status) VALUES (?,?,?,?,'2026-06-01 10:00:00','2026-06-01 12:00:00','pickup','Completed')`,
		authorID, offID, catID, addrID)
	var ticketID uint64
	db.QueryRow(`SELECT id FROM tickets WHERE user_id=?`, authorID).Scan(&ticketID)
	db.Exec(`INSERT INTO reviews (ticket_id, user_id, offering_id, rating, text, status)
		VALUES (?,?,?,?,?, 'pending_moderation')`,
		ticketID, authorID, offID, 4, "review text")
	var reviewID uint64
	db.QueryRow(`SELECT id FROM reviews WHERE ticket_id=?`, ticketID).Scan(&reviewID)

	svc := moderation.NewService(db)
	item, err := svc.EnqueueBorderline(context.Background(), models.ModContentReview, reviewID, "review text", []string{"text"})
	require.NoError(t, err)

	updated, err := svc.ApproveItem(context.Background(), item.ID, modID, "looks fine")
	require.NoError(t, err)
	assert.Equal(t, models.ModStatusApproved, updated.Status)

	// Underlying review should be promoted back to published
	var status string
	db.QueryRow(`SELECT status FROM reviews WHERE id=?`, reviewID).Scan(&status)
	assert.Equal(t, "published", status)

	// No freeze on approve
	until, _ := svc.IsUserFrozen(context.Background(), authorID)
	assert.Nil(t, until)
}

func TestRejectItem_AlreadyReviewed_Rejected(t *testing.T) {
	db := testutil.DBOrSkip(t)
	setupModTables(t)

	// Seed minimal data
	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?),(?,?,?,?)`,
		"x_a", "xa@t.l", "$2a$04$p", "XA",
		"x_m", "xm@t.l", "$2a$04$p", "XM")
	var aID, mID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='x_a'`).Scan(&aID)
	db.QueryRow(`SELECT id FROM users WHERE username='x_m'`).Scan(&mID)

	db.Exec(`INSERT INTO service_categories (name, slug, response_time_minutes) VALUES (?,?,?)`, "XC", "xc", 60)
	var catID uint64
	db.QueryRow(`SELECT id FROM service_categories WHERE slug='xc'`).Scan(&catID)
	db.Exec(`INSERT INTO service_offerings (agent_id, category_id, name, duration_minutes) VALUES (?,?,?,?)`,
		aID, catID, "XO", 60)
	var offID uint64
	db.QueryRow(`SELECT id FROM service_offerings WHERE name='XO'`).Scan(&offID)
	db.Exec(`INSERT INTO addresses (user_id, label, address_line1_encrypted, city, state, zip, is_default) VALUES (?,?,?,?,?,?,1)`,
		aID, "H", []byte("1 Main"), "X", "NY", "10001")
	var addrID uint64
	db.QueryRow(`SELECT id FROM addresses WHERE user_id=?`, aID).Scan(&addrID)
	db.Exec(`INSERT INTO tickets (user_id, offering_id, category_id, address_id, preferred_start, preferred_end,
		delivery_method, status) VALUES (?,?,?,?,'2026-06-01 10:00:00','2026-06-01 12:00:00','pickup','Completed')`,
		aID, offID, catID, addrID)
	var ticketID uint64
	db.QueryRow(`SELECT id FROM tickets WHERE user_id=?`, aID).Scan(&ticketID)
	db.Exec(`INSERT INTO reviews (ticket_id, user_id, offering_id, rating, status)
		VALUES (?,?,?,?, 'pending_moderation')`, ticketID, aID, offID, 3)
	var reviewID uint64
	db.QueryRow(`SELECT id FROM reviews WHERE ticket_id=?`, ticketID).Scan(&reviewID)

	svc := moderation.NewService(db)
	item, _ := svc.EnqueueBorderline(context.Background(), models.ModContentReview, reviewID, "x", []string{"x"})

	// Approve once
	_, err := svc.ApproveItem(context.Background(), item.ID, mID, "ok")
	require.NoError(t, err)

	// Try to reject the same item — should error with validation
	_, _, err = svc.RejectItem(context.Background(), item.ID, mID, "no")
	assert.ErrorIs(t, err, moderation.ErrValidation)
}

func TestIsUserFrozen_ExpiredFreeze_ReturnsNil(t *testing.T) {
	db := testutil.DBOrSkip(t)
	setupModTables(t)

	db.Exec(`INSERT INTO users (username, email, password_hash, display_name, posting_freeze_until)
		VALUES (?,?,?,?,?)`,
		"thawed", "tw@t.l", "$2a$04$p", "TW", time.Now().Add(-1*time.Hour))
	var userID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='thawed'`).Scan(&userID)

	svc := moderation.NewService(db)
	until, err := svc.IsUserFrozen(context.Background(), userID)
	require.NoError(t, err)
	assert.Nil(t, until, "past freeze deadline should return nil")
}

func TestListViolations_NewestFirst(t *testing.T) {
	db := testutil.DBOrSkip(t)
	setupModTables(t)

	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"history", "h@t.l", "$2a$04$p", "H")
	var userID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='history'`).Scan(&userID)

	for i := 0; i < 3; i++ {
		db.Exec(`INSERT INTO violation_records (user_id, content_type, content_id, freeze_applied, freeze_duration_hours)
			VALUES (?,?,?,?,?)`, userID, "review", uint64(i+1), true, 24)
	}

	svc := moderation.NewService(db)
	out, err := svc.ListViolations(context.Background(), userID)
	require.NoError(t, err)
	assert.Len(t, out, 3)
	// Newest first → highest id first
	assert.True(t, out[0].ID > out[1].ID)
}
