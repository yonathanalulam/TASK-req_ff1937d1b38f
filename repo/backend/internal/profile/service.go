package profile

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/eagle-point/service-portal/internal/crypto"
	"github.com/eagle-point/service-portal/internal/models"
)

// ErrNotFound is returned when the requested entity does not exist.
var ErrNotFound = errors.New("not found")

// Service provides profile, preferences, favorites, and browsing-history logic.
type Service struct {
	db     *sql.DB
	encKey string
}

// NewService creates a Service.
func NewService(db *sql.DB, encKey string) *Service {
	return &Service{db: db, encKey: encKey}
}

// ─── Profile ─────────────────────────────────────────────────────────────────

// ProfileView is the response shape for profile reads.
type ProfileView struct {
	ID          uint64    `json:"id"`
	Username    string    `json:"username"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	AvatarURL   string    `json:"avatar_url"`
	Bio         string    `json:"bio"`
	Phone       string    `json:"phone,omitempty"` // masked unless caller is admin
	Roles       []string  `json:"roles"`
	CreatedAt   time.Time `json:"created_at"`
}

// UpdateProfileInput carries validated profile update fields.
type UpdateProfileInput struct {
	DisplayName string
	AvatarURL   string
	Bio         string
	Phone       string // raw digits; empty = leave unchanged
}

// GetProfile returns the authenticated user's profile.
// isAdmin controls whether phone is returned unmasked.
func (s *Service) GetProfile(ctx context.Context, userID uint64, isAdmin bool) (*ProfileView, error) {
	var p ProfileView
	var phoneEnc []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, email, display_name, avatar_url, bio, phone_encrypted, created_at
		 FROM users WHERE id = ? AND is_deleted = 0`,
		userID,
	).Scan(&p.ID, &p.Username, &p.Email, &p.DisplayName,
		&p.AvatarURL, &p.Bio, &phoneEnc, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("profile.GetProfile: %w", err)
	}

	if len(phoneEnc) > 0 {
		raw, decErr := decryptField(phoneEnc, s.encKey)
		if decErr == nil {
			if isAdmin {
				p.Phone = raw
			} else {
				p.Phone = MaskPhone(raw)
			}
		}
	}

	p.Roles, _ = s.loadRoles(ctx, userID)
	return &p, nil
}

// UpdateProfile applies profile changes and returns the updated view.
func (s *Service) UpdateProfile(ctx context.Context, userID uint64, in UpdateProfileInput) (*ProfileView, error) {
	in.DisplayName = trim(in.DisplayName)
	if in.DisplayName == "" {
		return nil, errors.New("display_name is required")
	}
	if len(in.DisplayName) > 100 {
		return nil, errors.New("display_name must be 100 characters or fewer")
	}

	if in.Phone != "" {
		enc, err := encryptField(in.Phone, s.encKey)
		if err != nil {
			return nil, fmt.Errorf("profile.UpdateProfile: encrypt phone: %w", err)
		}
		_, err = s.db.ExecContext(ctx,
			`UPDATE users SET display_name=?, avatar_url=?, bio=?, phone_encrypted=? WHERE id=?`,
			in.DisplayName, in.AvatarURL, in.Bio, enc, userID,
		)
		if err != nil {
			return nil, fmt.Errorf("profile.UpdateProfile: %w", err)
		}
	} else {
		_, err := s.db.ExecContext(ctx,
			`UPDATE users SET display_name=?, avatar_url=?, bio=? WHERE id=?`,
			in.DisplayName, in.AvatarURL, in.Bio, userID,
		)
		if err != nil {
			return nil, fmt.Errorf("profile.UpdateProfile: %w", err)
		}
	}

	return s.GetProfile(ctx, userID, false)
}

// ─── Preferences ─────────────────────────────────────────────────────────────

// UpdatePreferencesInput carries validated preference fields.
type UpdatePreferencesInput struct {
	NotifyInApp  bool
	MutedTags    []int64
	MutedAuthors []int64
}

// GetPreferences returns the user's current preference settings.
func (s *Service) GetPreferences(ctx context.Context, userID uint64) (*models.Preferences, error) {
	p := &models.Preferences{UserID: userID}
	var tagsJSON, authorsJSON []byte

	err := s.db.QueryRowContext(ctx,
		`SELECT notify_in_app, muted_tags, muted_authors, updated_at
		 FROM user_preferences WHERE user_id = ?`,
		userID,
	).Scan(&p.NotifyInApp, &tagsJSON, &authorsJSON, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		// Row should always exist (created on registration) but handle gracefully.
		return &models.Preferences{UserID: userID, NotifyInApp: true}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("profile.GetPreferences: %w", err)
	}

	if len(tagsJSON) > 0 {
		_ = json.Unmarshal(tagsJSON, &p.MutedTags)
	}
	if p.MutedTags == nil {
		p.MutedTags = []int64{}
	}
	if len(authorsJSON) > 0 {
		_ = json.Unmarshal(authorsJSON, &p.MutedAuthors)
	}
	if p.MutedAuthors == nil {
		p.MutedAuthors = []int64{}
	}

	return p, nil
}

// UpdatePreferences replaces the user's preference settings.
func (s *Service) UpdatePreferences(ctx context.Context, userID uint64, in UpdatePreferencesInput) (*models.Preferences, error) {
	if in.MutedTags == nil {
		in.MutedTags = []int64{}
	}
	if in.MutedAuthors == nil {
		in.MutedAuthors = []int64{}
	}

	tagsJSON, _ := json.Marshal(in.MutedTags)
	authorsJSON, _ := json.Marshal(in.MutedAuthors)

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO user_preferences (user_id, notify_in_app, muted_tags, muted_authors)
		 VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE notify_in_app=VALUES(notify_in_app),
		                         muted_tags=VALUES(muted_tags),
		                         muted_authors=VALUES(muted_authors)`,
		userID, in.NotifyInApp, tagsJSON, authorsJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("profile.UpdatePreferences: %w", err)
	}

	return s.GetPreferences(ctx, userID)
}

// ─── Favorites ───────────────────────────────────────────────────────────────

// FavoriteItem represents a single favorites list entry.
type FavoriteItem struct {
	ID         uint64    `json:"id"`
	OfferingID uint64    `json:"offering_id"`
	CreatedAt  time.Time `json:"created_at"`
}

// FavoritePage is a paginated favorites response.
type FavoritePage struct {
	Items      []FavoriteItem `json:"items"`
	NextCursor uint64         `json:"next_cursor"` // 0 = no more pages
}

// ListFavorites returns a cursor-paginated list of the user's favorites.
// cursor=0 starts from the most recent entry.
func (s *Service) ListFavorites(ctx context.Context, userID, cursor uint64, limit int) (*FavoritePage, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, offering_id, created_at
		 FROM user_favorites
		 WHERE user_id = ? AND (? = 0 OR id < ?)
		 ORDER BY id DESC
		 LIMIT ?`,
		userID, cursor, cursor, limit+1,
	)
	if err != nil {
		return nil, fmt.Errorf("profile.ListFavorites: %w", err)
	}
	defer rows.Close()

	var items []FavoriteItem
	for rows.Next() {
		var f FavoriteItem
		if err := rows.Scan(&f.ID, &f.OfferingID, &f.CreatedAt); err != nil {
			return nil, fmt.Errorf("profile.ListFavorites: scan: %w", err)
		}
		items = append(items, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("profile.ListFavorites: rows: %w", err)
	}

	page := &FavoritePage{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextCursor = items[limit-1].ID
	}
	if page.Items == nil {
		page.Items = []FavoriteItem{}
	}
	return page, nil
}

// AddFavorite saves an offering to the user's favorites (idempotent).
func (s *Service) AddFavorite(ctx context.Context, userID, offeringID uint64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT IGNORE INTO user_favorites (user_id, offering_id) VALUES (?, ?)`,
		userID, offeringID,
	)
	if err != nil {
		return fmt.Errorf("profile.AddFavorite: %w", err)
	}
	return nil
}

// RemoveFavorite deletes an offering from the user's favorites.
func (s *Service) RemoveFavorite(ctx context.Context, userID, offeringID uint64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM user_favorites WHERE user_id = ? AND offering_id = ?`,
		userID, offeringID,
	)
	if err != nil {
		return fmt.Errorf("profile.RemoveFavorite: %w", err)
	}
	return nil
}

// ─── Browsing History ────────────────────────────────────────────────────────

// HistoryItem represents a single browsing-history entry.
type HistoryItem struct {
	ID         uint64    `json:"id"`
	OfferingID uint64    `json:"offering_id"`
	ViewedAt   time.Time `json:"viewed_at"`
}

// HistoryPage is a paginated history response.
type HistoryPage struct {
	Items      []HistoryItem `json:"items"`
	NextCursor uint64        `json:"next_cursor"`
}

// ListHistory returns a cursor-paginated browsing history list.
func (s *Service) ListHistory(ctx context.Context, userID, cursor uint64, limit int) (*HistoryPage, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, offering_id, viewed_at
		 FROM user_browsing_history
		 WHERE user_id = ? AND (? = 0 OR id < ?)
		 ORDER BY id DESC
		 LIMIT ?`,
		userID, cursor, cursor, limit+1,
	)
	if err != nil {
		return nil, fmt.Errorf("profile.ListHistory: %w", err)
	}
	defer rows.Close()

	var items []HistoryItem
	for rows.Next() {
		var h HistoryItem
		if err := rows.Scan(&h.ID, &h.OfferingID, &h.ViewedAt); err != nil {
			return nil, fmt.Errorf("profile.ListHistory: scan: %w", err)
		}
		items = append(items, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("profile.ListHistory: rows: %w", err)
	}

	page := &HistoryPage{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextCursor = items[limit-1].ID
	}
	if page.Items == nil {
		page.Items = []HistoryItem{}
	}
	return page, nil
}

// ClearHistory removes all browsing history entries for the user.
func (s *Service) ClearHistory(ctx context.Context, userID uint64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM user_browsing_history WHERE user_id = ?`, userID)
	if err != nil {
		return fmt.Errorf("profile.ClearHistory: %w", err)
	}
	return nil
}

// RecordView inserts a browsing-history entry. Called by Phase 4 offering handler.
func (s *Service) RecordView(ctx context.Context, userID, offeringID uint64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO user_browsing_history (user_id, offering_id) VALUES (?, ?)`,
		userID, offeringID,
	)
	if err != nil {
		return fmt.Errorf("profile.RecordView: %w", err)
	}
	return nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func (s *Service) loadRoles(ctx context.Context, userID uint64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT r.name FROM roles r JOIN user_roles ur ON ur.role_id = r.id WHERE ur.user_id = ?`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var roles []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		roles = append(roles, name)
	}
	return roles, rows.Err()
}

// encryptField encrypts a string value.
// When encKey is empty (test mode) the raw bytes are returned unchanged.
func encryptField(value, encKey string) ([]byte, error) {
	if encKey == "" {
		return []byte(value), nil
	}
	return crypto.EncryptString(value, encKey)
}

// decryptField decrypts a byte slice into a string.
// When encKey is empty (test mode) the raw bytes are returned as a string.
func decryptField(data []byte, encKey string) (string, error) {
	if encKey == "" || len(data) == 0 {
		return string(data), nil
	}
	return crypto.DecryptString(data, encKey)
}

func trim(s string) string {
	return strings.TrimSpace(s)
}
