package session

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/eagle-point/service-portal/internal/models"
)

const (
	// InactivityTimeout is the duration of inactivity before a session is invalidated.
	InactivityTimeout = 30 * time.Minute
	// AbsoluteTimeout is the maximum session lifetime regardless of activity.
	AbsoluteTimeout = 24 * time.Hour

	cookieName = "sp_session"
)

// Store handles session persistence against the sessions table.
type Store struct {
	db *sql.DB
}

// New creates a new session Store.
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// Create inserts a new session and returns it.
func (s *Store) Create(ctx context.Context, userID uint64, ip, ua string) (*models.Session, error) {
	id := uuid.New().String()
	csrf, err := generateToken(32)
	if err != nil {
		return nil, fmt.Errorf("session.Create: generate csrf: %w", err)
	}

	now := time.Now()
	sess := &models.Session{
		ID:           id,
		UserID:       userID,
		CSRFToken:    csrf,
		IPAddress:    ip,
		UserAgent:    ua,
		LastActiveAt: now,
		ExpiresAt:    now.Add(AbsoluteTimeout),
		CreatedAt:    now,
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO sessions (id, user_id, csrf_token, ip_address, user_agent, last_active_at, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.ID, sess.UserID, sess.CSRFToken, sess.IPAddress, sess.UserAgent,
		sess.LastActiveAt, sess.ExpiresAt, sess.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("session.Create: insert: %w", err)
	}

	return sess, nil
}

// GetByID loads a session by its ID. Returns nil, nil if not found.
func (s *Store) GetByID(ctx context.Context, id string) (*models.Session, error) {
	var sess models.Session
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, csrf_token, ip_address, user_agent, last_active_at, expires_at, created_at
		 FROM sessions WHERE id = ?`, id,
	).Scan(
		&sess.ID, &sess.UserID, &sess.CSRFToken, &sess.IPAddress, &sess.UserAgent,
		&sess.LastActiveAt, &sess.ExpiresAt, &sess.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("session.GetByID: %w", err)
	}
	return &sess, nil
}

// Touch updates last_active_at to now. Call this on every authenticated request.
func (s *Store) Touch(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET last_active_at = ? WHERE id = ?`,
		time.Now(), id,
	)
	if err != nil {
		return fmt.Errorf("session.Touch: %w", err)
	}
	return nil
}

// Delete removes a session by ID (logout).
func (s *Store) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("session.Delete: %w", err)
	}
	return nil
}

// DeleteAllForUser removes all sessions for a user (force logout all devices).
func (s *Store) DeleteAllForUser(ctx context.Context, userID uint64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	if err != nil {
		return fmt.Errorf("session.DeleteAllForUser: %w", err)
	}
	return nil
}

// PurgeExpired removes all sessions that have passed their absolute expiry.
func (s *Store) PurgeExpired(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < NOW()`)
	return err
}

// CookieName returns the session cookie name.
func CookieName() string { return cookieName }

func generateToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
