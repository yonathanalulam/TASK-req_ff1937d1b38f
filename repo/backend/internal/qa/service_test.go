package qa_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eagle-point/service-portal/internal/qa"
	"github.com/eagle-point/service-portal/internal/testutil"
)

// ─── Integration: thread CRUD ────────────────────────────────────────────────

func TestQA_CreateAndListThreads(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"qa_posts", "qa_threads",
		"service_offerings", "service_categories",
		"login_attempts", "sessions", "user_roles", "users",
	)

	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"qauser", "qa@t.l", "$2a$04$p", "Q")
	var userID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='qauser'`).Scan(&userID)
	db.Exec(`INSERT INTO service_categories (name, slug, response_time_minutes) VALUES (?,?,?)`, "QC", "qc", 60)
	var catID uint64
	db.QueryRow(`SELECT id FROM service_categories WHERE slug='qc'`).Scan(&catID)
	db.Exec(`INSERT INTO service_offerings (agent_id, category_id, name, duration_minutes) VALUES (?,?,?,?)`,
		userID, catID, "QO", 60)
	var offID uint64
	db.QueryRow(`SELECT id FROM service_offerings WHERE name='QO'`).Scan(&offID)

	svc := qa.NewService(db)
	ctx := context.Background()

	// Create two threads
	t1, err := svc.CreateThread(ctx, offID, userID, "How long does this take?")
	require.NoError(t, err)
	assert.Equal(t, "How long does this take?", t1.Question)
	assert.Equal(t, "published", t1.Status)

	_, err = svc.CreateThread(ctx, offID, userID, "Do you serve weekends?")
	require.NoError(t, err)

	// List
	threads, next, err := svc.ListThreads(ctx, offID, 0, 10)
	require.NoError(t, err)
	assert.Len(t, threads, 2)
	assert.Equal(t, uint64(0), next, "no more pages expected")
	// Newest first
	assert.Equal(t, "Do you serve weekends?", threads[0].Question)
}

func TestQA_CreateThread_EmptyQuestion_Rejected(t *testing.T) {
	db := testutil.DBOrSkip(t)
	svc := qa.NewService(db)
	_, err := svc.CreateThread(context.Background(), 1, 1, "   ")
	assert.ErrorIs(t, err, qa.ErrValidation)
}

func TestQA_CreateThread_MissingOffering(t *testing.T) {
	db := testutil.DBOrSkip(t)
	svc := qa.NewService(db)
	_, err := svc.CreateThread(context.Background(), 0, 1, "valid question")
	assert.ErrorIs(t, err, qa.ErrValidation)
}

// ─── Integration: replies ────────────────────────────────────────────────────

func TestQA_CreateReply_AppendsToThread(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"qa_posts", "qa_threads",
		"service_offerings", "service_categories",
		"login_attempts", "sessions", "user_roles", "users",
	)

	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?),(?,?,?,?)`,
		"asker", "ask@t.l", "$2a$04$p", "Asker",
		"agent", "ag@t.l", "$2a$04$p", "Agent")
	var askerID, agentID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='asker'`).Scan(&askerID)
	db.QueryRow(`SELECT id FROM users WHERE username='agent'`).Scan(&agentID)

	db.Exec(`INSERT INTO service_categories (name, slug, response_time_minutes) VALUES (?,?,?)`, "RC", "qrc", 60)
	var catID uint64
	db.QueryRow(`SELECT id FROM service_categories WHERE slug='qrc'`).Scan(&catID)
	db.Exec(`INSERT INTO service_offerings (agent_id, category_id, name, duration_minutes) VALUES (?,?,?,?)`,
		agentID, catID, "RO", 60)
	var offID uint64
	db.QueryRow(`SELECT id FROM service_offerings WHERE name='RO'`).Scan(&offID)

	svc := qa.NewService(db)
	ctx := context.Background()

	thread, _ := svc.CreateThread(ctx, offID, askerID, "Question?")

	reply, err := svc.CreateReply(ctx, thread.ID, agentID, "Here is the answer.")
	require.NoError(t, err)
	assert.Equal(t, thread.ID, reply.ThreadID)
	assert.Equal(t, "published", reply.Status)

	// Fetch thread with replies
	full, err := svc.GetThread(ctx, thread.ID)
	require.NoError(t, err)
	assert.Len(t, full.Replies, 1)
	assert.Equal(t, "Here is the answer.", full.Replies[0].Content)
}

func TestQA_CreateReply_NonexistentThread(t *testing.T) {
	db := testutil.DBOrSkip(t)
	svc := qa.NewService(db)
	_, err := svc.CreateReply(context.Background(), 999999, 1, "hello")
	assert.ErrorIs(t, err, qa.ErrNotFound)
}

func TestQA_CreateReply_EmptyContent_Rejected(t *testing.T) {
	db := testutil.DBOrSkip(t)
	svc := qa.NewService(db)
	_, err := svc.CreateReply(context.Background(), 1, 1, "  ")
	assert.ErrorIs(t, err, qa.ErrValidation)
}

// ─── Integration: delete post ────────────────────────────────────────────────

func TestQA_DeletePost_HidesFromList(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"qa_posts", "qa_threads",
		"service_offerings", "service_categories",
		"login_attempts", "sessions", "user_roles", "users",
	)

	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?),(?,?,?,?)`,
		"d_a", "da@t.l", "$2a$04$p", "DA",
		"d_b", "db@t.l", "$2a$04$p", "DB")
	var aID, bID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='d_a'`).Scan(&aID)
	db.QueryRow(`SELECT id FROM users WHERE username='d_b'`).Scan(&bID)
	db.Exec(`INSERT INTO service_categories (name, slug, response_time_minutes) VALUES (?,?,?)`, "DC", "qdel", 60)
	var catID uint64
	db.QueryRow(`SELECT id FROM service_categories WHERE slug='qdel'`).Scan(&catID)
	db.Exec(`INSERT INTO service_offerings (agent_id, category_id, name, duration_minutes) VALUES (?,?,?,?)`,
		bID, catID, "DO", 60)
	var offID uint64
	db.QueryRow(`SELECT id FROM service_offerings WHERE name='DO'`).Scan(&offID)

	svc := qa.NewService(db)
	ctx := context.Background()

	thread, _ := svc.CreateThread(ctx, offID, aID, "?")
	reply, _ := svc.CreateReply(ctx, thread.ID, bID, "answer one")
	_, _ = svc.CreateReply(ctx, thread.ID, bID, "answer two")

	require.NoError(t, svc.DeletePost(ctx, reply.ID))

	full, err := svc.GetThread(ctx, thread.ID)
	require.NoError(t, err)
	assert.Len(t, full.Replies, 1, "removed reply must not appear")
	assert.Equal(t, "answer two", full.Replies[0].Content)
}

func TestQA_DeletePost_NotFound(t *testing.T) {
	db := testutil.DBOrSkip(t)
	svc := qa.NewService(db)
	err := svc.DeletePost(context.Background(), 999999)
	assert.ErrorIs(t, err, qa.ErrNotFound)
}

// ─── Integration: pagination ─────────────────────────────────────────────────

func TestQA_ListThreads_Pagination(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db,
		"qa_posts", "qa_threads",
		"service_offerings", "service_categories",
		"login_attempts", "sessions", "user_roles", "users",
	)

	db.Exec(`INSERT INTO users (username, email, password_hash, display_name) VALUES (?,?,?,?)`,
		"pgu", "pg@t.l", "$2a$04$p", "PG")
	var userID uint64
	db.QueryRow(`SELECT id FROM users WHERE username='pgu'`).Scan(&userID)
	db.Exec(`INSERT INTO service_categories (name, slug, response_time_minutes) VALUES (?,?,?)`, "PC", "pgcat", 60)
	var catID uint64
	db.QueryRow(`SELECT id FROM service_categories WHERE slug='pgcat'`).Scan(&catID)
	db.Exec(`INSERT INTO service_offerings (agent_id, category_id, name, duration_minutes) VALUES (?,?,?,?)`,
		userID, catID, "PGO", 60)
	var offID uint64
	db.QueryRow(`SELECT id FROM service_offerings WHERE name='PGO'`).Scan(&offID)

	svc := qa.NewService(db)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		svc.CreateThread(ctx, offID, userID, "q")
	}

	page1, next, err := svc.ListThreads(ctx, offID, 0, 3)
	require.NoError(t, err)
	assert.Len(t, page1, 3)
	assert.NotZero(t, next)

	page2, next2, err := svc.ListThreads(ctx, offID, next, 3)
	require.NoError(t, err)
	assert.Len(t, page2, 2)
	assert.Zero(t, next2)
}
