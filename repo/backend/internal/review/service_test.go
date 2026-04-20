package review_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eagle-point/service-portal/internal/review"
	"github.com/eagle-point/service-portal/internal/testutil"
)

// ─── Integration: review creation ─────────────────────────────────────────────

func TestReview_Create_Success(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"review_reports", "review_images", "reviews",
		"ticket_attachments", "ticket_notes", "tickets",
		"service_offerings", "service_categories",
		"addresses", "login_attempts", "sessions", "user_roles", "users",
	)

	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"revuser", "r@t.l", "$2a$04$p", "R")
	var userID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='revuser'`).Scan(&userID)

	db.Exec(`INSERT INTO service_categories (name, slug, response_time_minutes) VALUES (?,?,?)`, "RC", "rc", 60)
	var catID uint64
	db.QueryRow(`SELECT id FROM service_categories WHERE slug='rc'`).Scan(&catID)

	db.Exec(`INSERT INTO service_offerings (agent_id, category_id, name, duration_minutes) VALUES (?,?,?,?)`,
		userID, catID, "RO", 60)
	var offID uint64
	db.QueryRow(`SELECT id FROM service_offerings WHERE name='RO'`).Scan(&offID)

	db.Exec(`INSERT INTO addresses (user_id, label, address_line1_encrypted, city, state, zip, is_default) VALUES (?,?,?,?,?,?,1)`,
		userID, "H", []byte("1 Main"), "X", "NY", "10001")
	var addrID uint64
	db.QueryRow(`SELECT id FROM addresses WHERE user_id=?`, userID).Scan(&addrID)

	db.Exec(
		`INSERT INTO tickets (user_id, offering_id, category_id, address_id, preferred_start, preferred_end,
		 delivery_method, status) VALUES (?,?,?,?,?,?, 'pickup', 'Completed')`,
		userID, offID, catID, addrID,
		time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour),
	)
	var ticketID uint64
	db.QueryRow(`SELECT id FROM tickets WHERE user_id=?`, userID).Scan(&ticketID)

	svc := review.NewService(db, "")
	r, err := svc.Create(context.Background(), review.CreateInput{
		TicketID: ticketID,
		UserID:   userID,
		Rating:   5,
		Text:     "Excellent service!",
	})
	require.NoError(t, err)
	assert.Equal(t, 5, r.Rating)
	assert.Equal(t, "Excellent service!", r.Text)
	assert.Equal(t, "published", r.Status)
}

func TestReview_Create_NonCompletedTicket_Rejected(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"review_reports", "review_images", "reviews",
		"ticket_attachments", "ticket_notes", "tickets",
		"service_offerings", "service_categories",
		"addresses", "login_attempts", "sessions", "user_roles", "users",
	)

	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"revnc", "rnc@t.l", "$2a$04$p", "NC")
	var userID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='revnc'`).Scan(&userID)
	db.Exec(`INSERT INTO service_categories (name, slug, response_time_minutes) VALUES (?,?,?)`, "RNC", "rnc", 60)
	var catID uint64
	db.QueryRow(`SELECT id FROM service_categories WHERE slug='rnc'`).Scan(&catID)
	db.Exec(`INSERT INTO service_offerings (agent_id, category_id, name, duration_minutes) VALUES (?,?,?,?)`,
		userID, catID, "RNCO", 60)
	var offID uint64
	db.QueryRow(`SELECT id FROM service_offerings WHERE name='RNCO'`).Scan(&offID)
	db.Exec(`INSERT INTO addresses (user_id, label, address_line1_encrypted, city, state, zip, is_default) VALUES (?,?,?,?,?,?,1)`,
		userID, "H", []byte("1 Main"), "X", "NY", "10001")
	var addrID uint64
	db.QueryRow(`SELECT id FROM addresses WHERE user_id=?`, userID).Scan(&addrID)

	// Ticket still Accepted (not Completed)
	db.Exec(
		`INSERT INTO tickets (user_id, offering_id, category_id, address_id, preferred_start, preferred_end,
		 delivery_method, status) VALUES (?,?,?,?,?,?, 'pickup', 'Accepted')`,
		userID, offID, catID, addrID,
		time.Now().Add(time.Hour), time.Now().Add(2*time.Hour),
	)
	var ticketID uint64
	db.QueryRow(`SELECT id FROM tickets WHERE user_id=?`, userID).Scan(&ticketID)

	svc := review.NewService(db, "")
	_, err := svc.Create(context.Background(), review.CreateInput{
		TicketID: ticketID, UserID: userID, Rating: 5, Text: "",
	})
	assert.ErrorIs(t, err, review.ErrNotEligible)
}

func TestReview_Create_NonOwner_Forbidden(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"review_reports", "review_images", "reviews",
		"ticket_attachments", "ticket_notes", "tickets",
		"service_offerings", "service_categories",
		"addresses", "login_attempts", "sessions", "user_roles", "users",
	)

	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?),(?,?,?,?)`,
		"owner1", "o1@t.l", "$2a$04$p", "O1",
		"other1", "x1@t.l", "$2a$04$p", "X1")
	var ownerID, otherID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='owner1'`).Scan(&ownerID)
	db.QueryRow(`SELECT id FROM users WHERE username='other1'`).Scan(&otherID)

	db.Exec(`INSERT INTO service_categories (name, slug, response_time_minutes) VALUES (?,?,?)`, "FR", "fr-cat", 60)
	var catID uint64
	db.QueryRow(`SELECT id FROM service_categories WHERE slug='fr-cat'`).Scan(&catID)
	db.Exec(`INSERT INTO service_offerings (agent_id, category_id, name, duration_minutes) VALUES (?,?,?,?)`,
		ownerID, catID, "FRO", 60)
	var offID uint64
	db.QueryRow(`SELECT id FROM service_offerings WHERE name='FRO'`).Scan(&offID)
	db.Exec(`INSERT INTO addresses (user_id, label, address_line1_encrypted, city, state, zip, is_default) VALUES (?,?,?,?,?,?,1)`,
		ownerID, "H", []byte("1 Main"), "X", "NY", "10001")
	var addrID uint64
	db.QueryRow(`SELECT id FROM addresses WHERE user_id=?`, ownerID).Scan(&addrID)

	db.Exec(
		`INSERT INTO tickets (user_id, offering_id, category_id, address_id, preferred_start, preferred_end,
		 delivery_method, status) VALUES (?,?,?,?,?,?, 'pickup', 'Completed')`,
		ownerID, offID, catID, addrID,
		time.Now().Add(-time.Hour), time.Now().Add(-30*time.Minute),
	)
	var ticketID uint64
	db.QueryRow(`SELECT id FROM tickets WHERE user_id=?`, ownerID).Scan(&ticketID)

	svc := review.NewService(db, "")
	_, err := svc.Create(context.Background(), review.CreateInput{
		TicketID: ticketID, UserID: otherID, Rating: 5,
	})
	assert.ErrorIs(t, err, review.ErrForbidden)
}

func TestReview_Create_Duplicate_Rejected(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"review_reports", "review_images", "reviews",
		"ticket_attachments", "ticket_notes", "tickets",
		"service_offerings", "service_categories",
		"addresses", "login_attempts", "sessions", "user_roles", "users",
	)

	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"dupuser", "dup@t.l", "$2a$04$p", "Dup")
	var userID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='dupuser'`).Scan(&userID)
	db.Exec(`INSERT INTO service_categories (name, slug, response_time_minutes) VALUES (?,?,?)`, "DC", "dc", 60)
	var catID uint64
	db.QueryRow(`SELECT id FROM service_categories WHERE slug='dc'`).Scan(&catID)
	db.Exec(`INSERT INTO service_offerings (agent_id, category_id, name, duration_minutes) VALUES (?,?,?,?)`,
		userID, catID, "DO", 60)
	var offID uint64
	db.QueryRow(`SELECT id FROM service_offerings WHERE name='DO'`).Scan(&offID)
	db.Exec(`INSERT INTO addresses (user_id, label, address_line1_encrypted, city, state, zip, is_default) VALUES (?,?,?,?,?,?,1)`,
		userID, "H", []byte("1 Main"), "X", "NY", "10001")
	var addrID uint64
	db.QueryRow(`SELECT id FROM addresses WHERE user_id=?`, userID).Scan(&addrID)
	db.Exec(
		`INSERT INTO tickets (user_id, offering_id, category_id, address_id, preferred_start, preferred_end,
		 delivery_method, status) VALUES (?,?,?,?,?,?, 'pickup', 'Completed')`,
		userID, offID, catID, addrID,
		time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour),
	)
	var ticketID uint64
	db.QueryRow(`SELECT id FROM tickets WHERE user_id=?`, userID).Scan(&ticketID)

	svc := review.NewService(db, "")
	_, err := svc.Create(context.Background(), review.CreateInput{
		TicketID: ticketID, UserID: userID, Rating: 4,
	})
	require.NoError(t, err)

	// Second attempt
	_, err = svc.Create(context.Background(), review.CreateInput{
		TicketID: ticketID, UserID: userID, Rating: 5,
	})
	assert.ErrorIs(t, err, review.ErrAlreadyExists)
}

func TestReview_Create_InvalidRating(t *testing.T) {
	db := testutil.DBOrSkip(t)
	svc := review.NewService(db, "")
	_, err := svc.Create(context.Background(), review.CreateInput{
		TicketID: 1, UserID: 1, Rating: 6,
	})
	assert.ErrorIs(t, err, review.ErrValidation)

	_, err = svc.Create(context.Background(), review.CreateInput{
		TicketID: 1, UserID: 1, Rating: 0,
	})
	assert.ErrorIs(t, err, review.ErrValidation)
}

// ─── Integration: summary calculation ────────────────────────────────────────

func TestReview_Summary_Math(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"review_reports", "review_images", "reviews",
		"ticket_attachments", "ticket_notes", "tickets",
		"service_offerings", "service_categories",
		"addresses", "login_attempts", "sessions", "user_roles", "users",
	)

	// Seed an offering and 4 reviews: ratings 5,4,3,2 → avg 3.5, positive=2 (ratings≥4), rate=0.5
	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"sumu", "su@t.l", "$2a$04$p", "SU")
	var userID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='sumu'`).Scan(&userID)
	db.Exec(`INSERT INTO service_categories (name, slug, response_time_minutes) VALUES (?,?,?)`, "SumC", "sum-c", 60)
	var catID uint64
	db.QueryRow(`SELECT id FROM service_categories WHERE slug='sum-c'`).Scan(&catID)
	db.Exec(`INSERT INTO service_offerings (agent_id, category_id, name, duration_minutes) VALUES (?,?,?,?)`,
		userID, catID, "SumO", 60)
	var offID uint64
	db.QueryRow(`SELECT id FROM service_offerings WHERE name='SumO'`).Scan(&offID)

	// 4 dummy tickets (one per review, all Completed)
	db.Exec(`INSERT INTO addresses (user_id, label, address_line1_encrypted, city, state, zip, is_default) VALUES (?,?,?,?,?,?,1)`,
		userID, "H", []byte("1 Main"), "X", "NY", "10001")
	var addrID uint64
	db.QueryRow(`SELECT id FROM addresses WHERE user_id=?`, userID).Scan(&addrID)

	for i := 0; i < 4; i++ {
		db.Exec(
			`INSERT INTO tickets (user_id, offering_id, category_id, address_id, preferred_start, preferred_end,
			 delivery_method, status) VALUES (?,?,?,?,?,?, 'pickup', 'Completed')`,
			userID, offID, catID, addrID,
			time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour),
		)
	}
	rows, _ := db.Query(`SELECT id FROM tickets WHERE user_id=? ORDER BY id ASC`, userID)
	var ticketIDs []uint64
	for rows.Next() {
		var id uint64
		rows.Scan(&id)
		ticketIDs = append(ticketIDs, id)
	}
	rows.Close()

	ratings := []int{5, 4, 3, 2}
	for i, rating := range ratings {
		db.Exec(
			`INSERT INTO reviews (ticket_id, user_id, offering_id, rating, status)
			 VALUES (?,?,?,?, 'published')`,
			ticketIDs[i], userID, offID, rating,
		)
	}

	svc := review.NewService(db, "")
	sum, err := svc.Summary(context.Background(), offID)
	require.NoError(t, err)
	assert.Equal(t, 4, sum.TotalReviews)
	assert.InDelta(t, 3.5, sum.AverageRating, 0.001)
	assert.InDelta(t, 0.5, sum.PositiveRate, 0.001)
}

func TestReview_Summary_Empty(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"review_reports", "review_images", "reviews",
		"ticket_attachments", "ticket_notes", "tickets",
		"service_offerings", "service_categories",
	)
	svc := review.NewService(db, "")
	sum, err := svc.Summary(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, 0, sum.TotalReviews)
	assert.Equal(t, 0.0, sum.AverageRating)
	assert.Equal(t, 0.0, sum.PositiveRate)
}

// ─── Integration: report creation ────────────────────────────────────────────

func TestReview_CreateReport_Success(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"review_reports", "review_images", "reviews",
		"ticket_attachments", "ticket_notes", "tickets",
		"service_offerings", "service_categories",
		"addresses", "login_attempts", "sessions", "user_roles", "users",
	)

	// Minimal seed — a user, a review
	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?),(?,?,?,?)`,
		"auth", "au@t.l", "$2a$04$p", "AU",
		"rep", "re@t.l", "$2a$04$p", "Re")
	var authorID, reporterID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='auth'`).Scan(&authorID)
	db.QueryRow(`SELECT id FROM users WHERE username='rep'`).Scan(&reporterID)

	db.Exec(`INSERT INTO service_categories (name, slug, response_time_minutes) VALUES (?,?,?)`, "RC2", "rc2", 60)
	var catID uint64
	db.QueryRow(`SELECT id FROM service_categories WHERE slug='rc2'`).Scan(&catID)
	db.Exec(`INSERT INTO service_offerings (agent_id, category_id, name, duration_minutes) VALUES (?,?,?,?)`,
		authorID, catID, "R2O", 60)
	var offID uint64
	db.QueryRow(`SELECT id FROM service_offerings WHERE name='R2O'`).Scan(&offID)
	db.Exec(`INSERT INTO addresses (user_id, label, address_line1_encrypted, city, state, zip, is_default) VALUES (?,?,?,?,?,?,1)`,
		authorID, "H", []byte("1 Main"), "X", "NY", "10001")
	var addrID uint64
	db.QueryRow(`SELECT id FROM addresses WHERE user_id=?`, authorID).Scan(&addrID)
	db.Exec(
		`INSERT INTO tickets (user_id, offering_id, category_id, address_id, preferred_start, preferred_end,
		 delivery_method, status) VALUES (?,?,?,?,?,?, 'pickup', 'Completed')`,
		authorID, offID, catID, addrID,
		time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour),
	)
	var ticketID uint64
	db.QueryRow(`SELECT id FROM tickets WHERE user_id=?`, authorID).Scan(&ticketID)
	db.Exec(
		`INSERT INTO reviews (ticket_id, user_id, offering_id, rating, text, status)
		 VALUES (?,?,?,?,?, 'published')`,
		ticketID, authorID, offID, 2, "bad",
	)
	var reviewID uint64
	db.QueryRow(`SELECT id FROM reviews WHERE ticket_id=?`, ticketID).Scan(&reviewID)

	svc := review.NewService(db, "")
	rpt, err := svc.CreateReport(context.Background(), reviewID, reporterID, "abusive", "contains slurs")
	require.NoError(t, err)
	assert.Equal(t, "abusive", rpt.Reason)
	assert.Equal(t, "contains slurs", rpt.Details)

	var n int
	db.QueryRow(`SELECT COUNT(*) FROM review_reports WHERE review_id=?`, reviewID).Scan(&n)
	assert.Equal(t, 1, n)
}

func TestReview_CreateReport_InvalidReason(t *testing.T) {
	db := testutil.DBOrSkip(t)
	svc := review.NewService(db, "")
	_, err := svc.CreateReport(context.Background(), 1, 1, "other", "")
	assert.ErrorIs(t, err, review.ErrValidation)
}
