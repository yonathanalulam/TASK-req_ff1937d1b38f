package qa

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/eagle-point/service-portal/internal/models"
)

// Errors.
var (
	ErrNotFound   = errors.New("not found")
	ErrForbidden  = errors.New("forbidden")
	ErrValidation = errors.New("validation error")
)

// Service provides Q&A business logic.
type Service struct {
	db *sql.DB
}

// NewService creates a Service.
func NewService(db *sql.DB) *Service { return &Service{db: db} }

// ─── Threads ─────────────────────────────────────────────────────────────────

// CreateThread inserts a new question thread.
func (s *Service) CreateThread(ctx context.Context, offeringID, authorID uint64, question string) (*models.QAThread, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return nil, fmt.Errorf("%w: question is required", ErrValidation)
	}
	if offeringID == 0 {
		return nil, fmt.Errorf("%w: offering_id is required", ErrValidation)
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO qa_threads (offering_id, author_id, question, status)
		 VALUES (?, ?, ?, 'published')`,
		offeringID, authorID, question,
	)
	if err != nil {
		return nil, fmt.Errorf("qa.CreateThread: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.GetThread(ctx, uint64(id))
}

// GetThread returns a thread with replies.
func (s *Service) GetThread(ctx context.Context, id uint64) (*models.QAThread, error) {
	t := &models.QAThread{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, offering_id, author_id, question, status, created_at, updated_at
		 FROM qa_threads WHERE id = ?`, id,
	).Scan(&t.ID, &t.OfferingID, &t.AuthorID, &t.Question, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	replies, err := s.listReplies(ctx, id)
	if err == nil {
		t.Replies = replies
	}
	return t, nil
}

// ListThreads returns paginated threads for an offering (newest first).
func (s *Service) ListThreads(ctx context.Context, offeringID, cursor uint64, limit int) ([]*models.QAThread, uint64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, offering_id, author_id, question, status, created_at, updated_at
		 FROM qa_threads
		 WHERE offering_id = ? AND status = 'published'
		   AND (? = 0 OR id < ?)
		 ORDER BY id DESC
		 LIMIT ?`,
		offeringID, cursor, cursor, limit+1,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("qa.ListThreads: %w", err)
	}
	defer rows.Close()

	var all []*models.QAThread
	for rows.Next() {
		t := &models.QAThread{}
		if err := rows.Scan(&t.ID, &t.OfferingID, &t.AuthorID, &t.Question, &t.Status,
			&t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, 0, err
		}
		all = append(all, t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	var next uint64
	if len(all) > limit {
		next = all[limit-1].ID
		all = all[:limit]
	}
	if all == nil {
		all = []*models.QAThread{}
	}

	// Load replies per thread
	for _, t := range all {
		if reps, err := s.listReplies(ctx, t.ID); err == nil {
			t.Replies = reps
		}
	}
	return all, next, nil
}

// ─── Replies (posts) ─────────────────────────────────────────────────────────

// CreateReply inserts a reply under a thread. Caller must be service_agent or administrator (enforced at handler).
func (s *Service) CreateReply(ctx context.Context, threadID, authorID uint64, content string) (*models.QAPost, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("%w: content is required", ErrValidation)
	}
	// Verify thread exists
	if _, err := s.GetThread(ctx, threadID); err != nil {
		return nil, err
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO qa_posts (thread_id, author_id, content, status)
		 VALUES (?, ?, ?, 'published')`,
		threadID, authorID, content,
	)
	if err != nil {
		return nil, fmt.Errorf("qa.CreateReply: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.getReply(ctx, uint64(id))
}

// DeletePost soft-removes a reply by flipping its status to 'removed'.
// Caller must hold moderator or administrator role (enforced at handler).
func (s *Service) DeletePost(ctx context.Context, postID uint64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE qa_posts SET status = 'removed' WHERE id = ? AND status != 'removed'`, postID)
	if err != nil {
		return fmt.Errorf("qa.DeletePost: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func (s *Service) listReplies(ctx context.Context, threadID uint64) ([]models.QAPost, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, thread_id, author_id, content, status, created_at, updated_at
		 FROM qa_posts
		 WHERE thread_id = ? AND status = 'published'
		 ORDER BY id ASC`, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.QAPost
	for rows.Next() {
		var p models.QAPost
		if err := rows.Scan(&p.ID, &p.ThreadID, &p.AuthorID, &p.Content, &p.Status,
			&p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Service) getReply(ctx context.Context, id uint64) (*models.QAPost, error) {
	p := &models.QAPost{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, thread_id, author_id, content, status, created_at, updated_at
		 FROM qa_posts WHERE id = ?`, id,
	).Scan(&p.ID, &p.ThreadID, &p.AuthorID, &p.Content, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}
