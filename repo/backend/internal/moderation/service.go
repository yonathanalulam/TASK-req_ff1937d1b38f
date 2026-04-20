// Package moderation implements the sensitive-term dictionary, content
// screening logic, moderation queue workflow, and violation/freeze
// enforcement engine described in Phase 8.
//
// The dictionary is backed by an in-memory cache that is rebuilt whenever a
// term is added or removed. Case-insensitive whole-word matching is applied
// to the submitted text.
package moderation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/eagle-point/service-portal/internal/models"
)

// Errors.
var (
	ErrNotFound   = errors.New("not found")
	ErrValidation = errors.New("validation error")
	ErrDuplicate  = errors.New("term already exists")
)

// Freeze escalation thresholds. Exposed as vars so tests can override.
var (
	FirstFreezeHours  = 24
	SecondFreezeHours = 24 * 7 // 7 days
)

// Service provides term dictionary, screening, queue, and freeze logic.
type Service struct {
	db *sql.DB

	cacheMu   sync.RWMutex
	cacheTerm map[string]string // lowered term → class
	cacheLoaded bool
}

// NewService creates a Service and loads the term cache on first use.
func NewService(db *sql.DB) *Service {
	return &Service{db: db, cacheTerm: make(map[string]string)}
}

// ─── Term cache ──────────────────────────────────────────────────────────────

// ReloadTerms rebuilds the in-memory dictionary cache from the database.
// Called automatically on Add/Delete and on first screening attempt.
func (s *Service) ReloadTerms(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT term, class FROM sensitive_terms`)
	if err != nil {
		return fmt.Errorf("moderation.ReloadTerms: %w", err)
	}
	defer rows.Close()

	m := make(map[string]string)
	for rows.Next() {
		var term, class string
		if err := rows.Scan(&term, &class); err != nil {
			return err
		}
		m[strings.ToLower(strings.TrimSpace(term))] = class
	}
	if err := rows.Err(); err != nil {
		return err
	}

	s.cacheMu.Lock()
	s.cacheTerm = m
	s.cacheLoaded = true
	s.cacheMu.Unlock()
	return nil
}

func (s *Service) ensureCache(ctx context.Context) {
	s.cacheMu.RLock()
	loaded := s.cacheLoaded
	s.cacheMu.RUnlock()
	if !loaded {
		_ = s.ReloadTerms(ctx)
	}
}

// ─── Term CRUD (admin) ───────────────────────────────────────────────────────

// ListTerms returns all dictionary entries.
func (s *Service) ListTerms(ctx context.Context) ([]*models.SensitiveTerm, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, term, class, created_at FROM sensitive_terms ORDER BY term ASC`)
	if err != nil {
		return nil, fmt.Errorf("moderation.ListTerms: %w", err)
	}
	defer rows.Close()
	var out []*models.SensitiveTerm
	for rows.Next() {
		t := &models.SensitiveTerm{}
		if err := rows.Scan(&t.ID, &t.Term, &t.Class, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if out == nil {
		out = []*models.SensitiveTerm{}
	}
	return out, rows.Err()
}

// AddTerm inserts a new dictionary entry and refreshes the cache.
func (s *Service) AddTerm(ctx context.Context, term, class string) (*models.SensitiveTerm, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("%w: term is required", ErrValidation)
	}
	if class != models.TermClassProhibited && class != models.TermClassBorderline {
		return nil, fmt.Errorf("%w: class must be prohibited or borderline", ErrValidation)
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO sensitive_terms (term, class) VALUES (?, ?)`, term, class,
	)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return nil, ErrDuplicate
		}
		return nil, fmt.Errorf("moderation.AddTerm: %w", err)
	}
	id, _ := res.LastInsertId()
	if err := s.ReloadTerms(ctx); err != nil {
		return nil, err
	}
	t := &models.SensitiveTerm{}
	err = s.db.QueryRowContext(ctx,
		`SELECT id, term, class, created_at FROM sensitive_terms WHERE id = ?`, id,
	).Scan(&t.ID, &t.Term, &t.Class, &t.CreatedAt)
	return t, err
}

// DeleteTerm removes a dictionary entry and refreshes the cache.
func (s *Service) DeleteTerm(ctx context.Context, id uint64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sensitive_terms WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("moderation.DeleteTerm: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return s.ReloadTerms(ctx)
}

// ─── Screening ───────────────────────────────────────────────────────────────

// ScreenResult captures the outcome of a content scan.
type ScreenResult struct {
	Class        string   // "" = clean, "prohibited", or "borderline"
	FlaggedTerms []string // terms that matched, always lowercased
}

// HasProhibited returns true if the scan hit any prohibited term.
func (r *ScreenResult) HasProhibited() bool { return r.Class == models.TermClassProhibited }

// HasBorderline returns true if at least one borderline term matched and no
// prohibited terms matched.
func (r *ScreenResult) HasBorderline() bool { return r.Class == models.TermClassBorderline }

// Screen scans `text` against the dictionary. A prohibited match beats a
// borderline match. Matching is case-insensitive, whole-word.
func (s *Service) Screen(ctx context.Context, text string) ScreenResult {
	s.ensureCache(ctx)

	s.cacheMu.RLock()
	cache := s.cacheTerm
	s.cacheMu.RUnlock()

	return ScanText(text, cache)
}

// ScanText is the pure-function core of Screen, exported for unit tests.
// `dict` maps lowered whole-term strings to class names.
func ScanText(text string, dict map[string]string) ScreenResult {
	r := ScreenResult{}
	if text == "" || len(dict) == 0 {
		return r
	}
	words := tokenize(text)
	seen := make(map[string]struct{})
	for _, w := range words {
		if class, ok := dict[w]; ok {
			if _, dup := seen[w]; dup {
				continue
			}
			seen[w] = struct{}{}
			r.FlaggedTerms = append(r.FlaggedTerms, w)
			if class == models.TermClassProhibited {
				r.Class = models.TermClassProhibited
			} else if r.Class != models.TermClassProhibited {
				r.Class = models.TermClassBorderline
			}
		}
	}
	return r
}

// tokenize splits text into lowercase whole-word tokens.
// Non-alphanumeric characters become delimiters.
func tokenize(text string) []string {
	text = strings.ToLower(text)
	var out []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range text {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return out
}

// ─── Moderation queue ────────────────────────────────────────────────────────

// OnBorderlineFlagged is the integration hook for content handlers. It marks
// the underlying content as pending_moderation and inserts a moderation queue
// row in a single transaction so the two states stay consistent.
func (s *Service) OnBorderlineFlagged(ctx context.Context, contentType string, contentID uint64, text string, flagged []string) error {
	// Tables that have a status column get their row demoted to pending_moderation.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := promoteContent(ctx, tx, contentType, contentID, "pending_moderation"); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	_, err = s.EnqueueBorderline(ctx, contentType, contentID, text, flagged)
	return err
}

// EnqueueBorderline inserts a borderline-matched item into the moderation queue.
func (s *Service) EnqueueBorderline(ctx context.Context, contentType string, contentID uint64, text string, flagged []string) (*models.ModerationQueueItem, error) {
	termsJSON, _ := json.Marshal(flagged)
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO moderation_queue (content_type, content_id, content_text, flagged_terms, status)
		 VALUES (?, ?, ?, ?, 'pending')`,
		contentType, contentID, text, termsJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("moderation.EnqueueBorderline: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.GetQueueItem(ctx, uint64(id))
}

// ListQueue returns pending queue items (newest first).
func (s *Service) ListQueue(ctx context.Context, status string) ([]*models.ModerationQueueItem, error) {
	if status == "" {
		status = models.ModStatusPending
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, content_type, content_id, content_text, flagged_terms, status,
		        moderator_id, reviewed_at, created_at
		 FROM moderation_queue WHERE status = ? ORDER BY id DESC`,
		status,
	)
	if err != nil {
		return nil, fmt.Errorf("moderation.ListQueue: %w", err)
	}
	defer rows.Close()

	var out []*models.ModerationQueueItem
	for rows.Next() {
		it, err := scanQueueItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	if out == nil {
		out = []*models.ModerationQueueItem{}
	}
	return out, rows.Err()
}

// GetQueueItem fetches a single queue row.
func (s *Service) GetQueueItem(ctx context.Context, id uint64) (*models.ModerationQueueItem, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, content_type, content_id, content_text, flagged_terms, status,
		        moderator_id, reviewed_at, created_at
		 FROM moderation_queue WHERE id = ?`, id)
	it, err := scanQueueItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return it, err
}

// ApproveItem marks a queue item approved and promotes the underlying content
// back to 'published'. Records a moderation_actions row.
func (s *Service) ApproveItem(ctx context.Context, itemID, moderatorID uint64, reason string) (*models.ModerationQueueItem, error) {
	it, err := s.GetQueueItem(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if it.Status != models.ModStatusPending {
		return nil, fmt.Errorf("%w: item already reviewed", ErrValidation)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`UPDATE moderation_queue SET status='approved', moderator_id=?, reviewed_at=NOW() WHERE id=?`,
		moderatorID, itemID,
	); err != nil {
		return nil, fmt.Errorf("moderation.ApproveItem: %w", err)
	}
	if err := promoteContent(ctx, tx, it.ContentType, it.ContentID, "published"); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO moderation_actions (moderator_id, action_type, content_type, content_id, reason)
		 VALUES (?, 'approve', ?, ?, ?)`,
		moderatorID, it.ContentType, it.ContentID, reason,
	); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetQueueItem(ctx, itemID)
}

// RejectItem marks a queue item rejected and records a violation against the
// content author, applying a posting freeze if thresholds are reached.
// Returns the updated queue item and the author's new freeze deadline (if any).
func (s *Service) RejectItem(ctx context.Context, itemID, moderatorID uint64, reason string) (*models.ModerationQueueItem, *time.Time, error) {
	it, err := s.GetQueueItem(ctx, itemID)
	if err != nil {
		return nil, nil, err
	}
	if it.Status != models.ModStatusPending {
		return nil, nil, fmt.Errorf("%w: item already reviewed", ErrValidation)
	}
	authorID, err := resolveAuthorID(ctx, s.db, it.ContentType, it.ContentID)
	if err != nil {
		return nil, nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`UPDATE moderation_queue SET status='rejected', moderator_id=?, reviewed_at=NOW() WHERE id=?`,
		moderatorID, itemID,
	); err != nil {
		return nil, nil, err
	}

	// Soft-delete / hide the content
	if err := promoteContent(ctx, tx, it.ContentType, it.ContentID, "rejected"); err != nil {
		return nil, nil, err
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO moderation_actions (moderator_id, action_type, content_type, content_id, reason)
		 VALUES (?, 'reject', ?, ?, ?)`,
		moderatorID, it.ContentType, it.ContentID, reason,
	); err != nil {
		return nil, nil, err
	}

	// Count prior violations (before this one)
	var priorCount int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM violation_records WHERE user_id = ?`, authorID,
	).Scan(&priorCount); err != nil {
		return nil, nil, err
	}
	newCount := priorCount + 1

	// Decide freeze escalation
	var freezeHours int
	switch newCount {
	case 1:
		freezeHours = FirstFreezeHours
	case 2:
		freezeHours = SecondFreezeHours
	default:
		// Keep applying the 7-day freeze on every subsequent strike
		freezeHours = SecondFreezeHours
	}

	var freezeUntil *time.Time
	if freezeHours > 0 {
		until := time.Now().UTC().Add(time.Duration(freezeHours) * time.Hour)
		if _, err := tx.ExecContext(ctx,
			`UPDATE users SET posting_freeze_until = ? WHERE id = ?`, until, authorID,
		); err != nil {
			return nil, nil, err
		}
		freezeUntil = &until
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO violation_records (user_id, content_type, content_id, freeze_applied, freeze_duration_hours)
		 VALUES (?, ?, ?, ?, ?)`,
		authorID, it.ContentType, it.ContentID, freezeHours > 0, freezeHours,
	); err != nil {
		return nil, nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	updated, err := s.GetQueueItem(ctx, itemID)
	return updated, freezeUntil, err
}

// ─── Actions log ─────────────────────────────────────────────────────────────

// ListActions returns moderation history (newest first), optionally per moderator.
func (s *Service) ListActions(ctx context.Context, moderatorID uint64, limit int) ([]*models.ModerationAction, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `SELECT id, moderator_id, action_type, content_type, content_id, reason, created_at
	      FROM moderation_actions`
	args := []interface{}{}
	if moderatorID > 0 {
		q += ` WHERE moderator_id = ?`
		args = append(args, moderatorID)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("moderation.ListActions: %w", err)
	}
	defer rows.Close()

	var out []*models.ModerationAction
	for rows.Next() {
		a := &models.ModerationAction{}
		var reason sql.NullString
		if err := rows.Scan(&a.ID, &a.ModeratorID, &a.ActionType, &a.ContentType,
			&a.ContentID, &reason, &a.CreatedAt); err != nil {
			return nil, err
		}
		if reason.Valid {
			a.Reason = reason.String
		}
		out = append(out, a)
	}
	if out == nil {
		out = []*models.ModerationAction{}
	}
	return out, rows.Err()
}

// ─── Violations ──────────────────────────────────────────────────────────────

// ListViolations returns violation history for a user (newest first).
func (s *Service) ListViolations(ctx context.Context, userID uint64) ([]*models.ViolationRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, content_type, content_id, violation_at, freeze_applied, freeze_duration_hours, created_at
		 FROM violation_records WHERE user_id = ? ORDER BY id DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("moderation.ListViolations: %w", err)
	}
	defer rows.Close()
	var out []*models.ViolationRecord
	for rows.Next() {
		v := &models.ViolationRecord{}
		if err := rows.Scan(&v.ID, &v.UserID, &v.ContentType, &v.ContentID,
			&v.ViolationAt, &v.FreezeApplied, &v.FreezeDurationHours, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if out == nil {
		out = []*models.ViolationRecord{}
	}
	return out, rows.Err()
}

// IsUserFrozen returns the freeze deadline if the user is currently frozen, else nil.
func (s *Service) IsUserFrozen(ctx context.Context, userID uint64) (*time.Time, error) {
	var until sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT posting_freeze_until FROM users WHERE id = ?`, userID,
	).Scan(&until)
	if err != nil {
		return nil, err
	}
	if !until.Valid {
		return nil, nil
	}
	if time.Now().Before(until.Time) {
		t := until.Time
		return &t, nil
	}
	return nil, nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

type queueScanner interface {
	Scan(dest ...any) error
}

func scanQueueItem(rs queueScanner) (*models.ModerationQueueItem, error) {
	it := &models.ModerationQueueItem{}
	var termsJSON sql.NullString
	var modID sql.NullInt64
	var reviewedAt sql.NullTime
	if err := rs.Scan(&it.ID, &it.ContentType, &it.ContentID, &it.ContentText,
		&termsJSON, &it.Status, &modID, &reviewedAt, &it.CreatedAt); err != nil {
		return nil, err
	}
	if termsJSON.Valid && termsJSON.String != "" {
		_ = json.Unmarshal([]byte(termsJSON.String), &it.FlaggedTerms)
	}
	if modID.Valid {
		id := uint64(modID.Int64)
		it.ModeratorID = &id
	}
	if reviewedAt.Valid {
		t := reviewedAt.Time
		it.ReviewedAt = &t
	}
	return it, nil
}

// promoteContent flips the status column of the underlying content row based on
// the content type. Unknown content types are a no-op (defensive).
func promoteContent(ctx context.Context, tx *sql.Tx, contentType string, contentID uint64, status string) error {
	switch contentType {
	case models.ModContentReview:
		_, err := tx.ExecContext(ctx, `UPDATE reviews SET status = ? WHERE id = ?`, status, contentID)
		return err
	case models.ModContentQAThread:
		s := status
		if status == "rejected" {
			s = "closed"
		}
		_, err := tx.ExecContext(ctx, `UPDATE qa_threads SET status = ? WHERE id = ?`, s, contentID)
		return err
	case models.ModContentQAPost:
		s := status
		if status == "rejected" {
			s = "removed"
		}
		_, err := tx.ExecContext(ctx, `UPDATE qa_posts SET status = ? WHERE id = ?`, s, contentID)
		return err
	case models.ModContentTicketNote:
		// ticket_notes has no status column — nothing to do
		return nil
	}
	return nil
}

// resolveAuthorID returns the author user_id for a given content item.
func resolveAuthorID(ctx context.Context, db *sql.DB, contentType string, contentID uint64) (uint64, error) {
	var authorCol, table string
	switch contentType {
	case models.ModContentReview:
		table, authorCol = "reviews", "user_id"
	case models.ModContentQAThread:
		table, authorCol = "qa_threads", "author_id"
	case models.ModContentQAPost:
		table, authorCol = "qa_posts", "author_id"
	case models.ModContentTicketNote:
		table, authorCol = "ticket_notes", "author_id"
	default:
		return 0, fmt.Errorf("%w: unknown content_type %q", ErrValidation, contentType)
	}
	var id uint64
	err := db.QueryRowContext(ctx, fmt.Sprintf(`SELECT %s FROM %s WHERE id = ?`, authorCol, table), contentID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}
