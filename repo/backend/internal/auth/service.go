package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"

	"github.com/eagle-point/service-portal/internal/models"
	"github.com/eagle-point/service-portal/internal/session"
)

// loginDebugEnabled is true when APP_ENV is development AND the
// AUTH_LOGIN_DEBUG environment variable is set. Production paths never emit
// login-metadata logs regardless of configuration.
func loginDebugEnabled() bool {
	return os.Getenv("APP_ENV") == "development" && os.Getenv("AUTH_LOGIN_DEBUG") != ""
}

const (
	bcryptCost         = 12
	lockoutFailures    = 5
	lockoutWindow      = 10 * time.Minute
	lockoutDuration    = 15 * time.Minute
)

// ErrInvalidCredentials is returned when username/password is wrong.
var ErrInvalidCredentials = errors.New("invalid username or password")

// LockoutError is returned when an account is locked.
type LockoutError struct {
	Until time.Time
}

func (e *LockoutError) Error() string {
	return fmt.Sprintf("account locked until %s", e.Until.Format(time.RFC3339))
}

func (e *LockoutError) RemainingSeconds() int {
	rem := time.Until(e.Until)
	if rem < 0 {
		return 0
	}
	return int(rem.Seconds()) + 1
}

// LoginResult is returned on a successful login.
type LoginResult struct {
	User    *models.User
	Session *session.Store
	Sess    interface{} // *models.Session — returned to handler
}

// LockoutNotifier is an optional hook invoked when an account transitions
// into the locked state. Kept as a function type to avoid importing the
// notification package from auth.
type LockoutNotifier func(ctx context.Context, userID uint64, until time.Time)

// Service provides authentication business logic.
type Service struct {
	db           *sql.DB
	sessionStore *session.Store
	onLockout    LockoutNotifier
}

// NewService creates an auth Service.
func NewService(db *sql.DB, ss *session.Store) *Service {
	return &Service{db: db, sessionStore: ss}
}

// SetLockoutNotifier wires an optional hook fired when an account is locked.
func (s *Service) SetLockoutNotifier(fn LockoutNotifier) { s.onLockout = fn }

// ─── Register ───────────────────────────────────────────────────────────────

type RegisterInput struct {
	Username    string
	Email       string
	Password    string
	DisplayName string
}

func (s *Service) Register(ctx context.Context, in RegisterInput) (*models.User, error) {
	in.Username = strings.TrimSpace(in.Username)
	in.Email = strings.TrimSpace(strings.ToLower(in.Email))
	in.DisplayName = strings.TrimSpace(in.DisplayName)

	if err := validatePassword(in.Password); err != nil {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("auth.Register: hash: %w", err)
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO users (username, email, password_hash, display_name) VALUES (?, ?, ?, ?)`,
		in.Username, in.Email, string(hash), in.DisplayName,
	)
	if err != nil {
		if isDuplicate(err) {
			return nil, errors.New("username or email already taken")
		}
		return nil, fmt.Errorf("auth.Register: insert: %w", err)
	}

	id, _ := res.LastInsertId()
	user := &models.User{
		ID:          uint64(id),
		Username:    in.Username,
		Email:       in.Email,
		DisplayName: in.DisplayName,
		IsActive:    true,
	}

	// Assign default role
	if err := s.assignRole(ctx, user.ID, models.RoleRegularUser); err != nil {
		return nil, fmt.Errorf("auth.Register: assign role: %w", err)
	}
	user.Roles = []string{models.RoleRegularUser}

	// Create default preferences
	_, _ = s.db.ExecContext(ctx,
		`INSERT IGNORE INTO user_preferences (user_id) VALUES (?)`, user.ID)

	return user, nil
}

// ─── Login ───────────────────────────────────────────────────────────────────

type LoginInput struct {
	Username  string
	Password  string
	IPAddress string
	UserAgent string
}

type LoginOutput struct {
	User    *models.User
	Session *models.Session
}

func (s *Service) Login(ctx context.Context, in LoginInput) (*LoginOutput, error) {
	// Check lockout before anything else
	if err := s.checkLockout(ctx, in.Username); err != nil {
		return nil, err
	}

	user, err := s.getUserByUsername(ctx, in.Username)
	if err != nil {
		if loginDebugEnabled() {
			log.Debug().Err(err).Msg("login: user lookup failed")
		}
		_ = s.recordAttempt(ctx, in.Username, in.IPAddress, false)
		return nil, ErrInvalidCredentials
	}
	if user == nil {
		if loginDebugEnabled() {
			log.Debug().Msg("login: user not found")
		}
		_ = s.recordAttempt(ctx, in.Username, in.IPAddress, false)
		return nil, ErrInvalidCredentials
	}

	if !user.IsActive || user.IsDeleted {
		if loginDebugEnabled() {
			log.Debug().Bool("active", user.IsActive).Bool("deleted", user.IsDeleted).Msg("login: account blocked")
		}
		_ = s.recordAttempt(ctx, in.Username, in.IPAddress, false)
		return nil, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(in.Password)); err != nil {
		if loginDebugEnabled() {
			// Dev-only: never log hash/password length metadata in production.
			log.Debug().Err(err).Msg("login: bcrypt mismatch")
		}
		_ = s.recordAttempt(ctx, in.Username, in.IPAddress, false)
		return nil, ErrInvalidCredentials
	}

	_ = s.recordAttempt(ctx, in.Username, in.IPAddress, true)

	sess, err := s.sessionStore.Create(ctx, user.ID, in.IPAddress, in.UserAgent)
	if err != nil {
		return nil, fmt.Errorf("auth.Login: create session: %w", err)
	}

	return &LoginOutput{User: user, Session: sess}, nil
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func (s *Service) GetUserByID(ctx context.Context, id uint64) (*models.User, error) {
	return s.getUserByID(ctx, id)
}

func (s *Service) getUserByUsername(ctx context.Context, username string) (*models.User, error) {
	var u models.User
	var avatarURL, bio sql.NullString
	var freeze sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, email, password_hash, display_name,
		        COALESCE(avatar_url,''), COALESCE(bio,''),
		        is_active, is_deleted, posting_freeze_until
		 FROM users WHERE username = ? AND is_deleted = 0`,
		username,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.DisplayName,
		&avatarURL, &bio, &u.IsActive, &u.IsDeleted, &freeze)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("auth: get user: %w", err)
	}
	if avatarURL.Valid {
		u.AvatarURL = avatarURL.String
	}
	if bio.Valid {
		u.Bio = bio.String
	}
	if freeze.Valid {
		u.PostingFreezeUntil = &freeze.Time
	}
	u.Roles, _ = s.loadRoles(ctx, u.ID)
	return &u, nil
}

func (s *Service) getUserByID(ctx context.Context, id uint64) (*models.User, error) {
	var u models.User
	var avatarURL, bio sql.NullString
	var freeze sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, email, display_name,
		        COALESCE(avatar_url,''), COALESCE(bio,''),
		        is_active, is_deleted, posting_freeze_until
		 FROM users WHERE id = ? AND is_deleted = 0`,
		id,
	).Scan(&u.ID, &u.Username, &u.Email, &u.DisplayName,
		&avatarURL, &bio, &u.IsActive, &u.IsDeleted, &freeze)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("auth: get user by id: %w", err)
	}
	if avatarURL.Valid {
		u.AvatarURL = avatarURL.String
	}
	if bio.Valid {
		u.Bio = bio.String
	}
	if freeze.Valid {
		u.PostingFreezeUntil = &freeze.Time
	}
	u.Roles, _ = s.loadRoles(ctx, u.ID)
	return &u, nil
}

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

func (s *Service) assignRole(ctx context.Context, userID uint64, roleName string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT IGNORE INTO user_roles (user_id, role_id)
		 SELECT ?, id FROM roles WHERE name = ?`,
		userID, roleName,
	)
	return err
}

func (s *Service) checkLockout(ctx context.Context, username string) error {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM login_attempts
		 WHERE username = ? AND success = 0
		 AND attempted_at > DATE_SUB(NOW(), INTERVAL ? SECOND)`,
		username, int(lockoutWindow.Seconds()),
	).Scan(&count)
	if err != nil || count < lockoutFailures {
		return nil
	}

	// Find when the lockout expires (based on most recent failure)
	var lastFailure time.Time
	err = s.db.QueryRowContext(ctx,
		`SELECT MAX(attempted_at) FROM login_attempts
		 WHERE username = ? AND success = 0
		 AND attempted_at > DATE_SUB(NOW(), INTERVAL ? SECOND)`,
		username, int(lockoutWindow.Seconds()),
	).Scan(&lastFailure)
	if err != nil {
		return nil
	}

	lockoutUntil := lastFailure.Add(lockoutDuration)
	if time.Now().Before(lockoutUntil) {
		return &LockoutError{Until: lockoutUntil}
	}
	return nil
}

func (s *Service) recordAttempt(ctx context.Context, username, ip string, success bool) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO login_attempts (username, ip_address, success) VALUES (?, ?, ?)`,
		username, ip, success,
	)
	if err != nil {
		return err
	}

	// Detect a lockout transition: this attempt failed and the failure count
	// just reached the threshold. Fire the notifier exactly once.
	if !success && s.onLockout != nil {
		var count int
		_ = s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM login_attempts
			 WHERE username = ? AND success = 0
			 AND attempted_at > DATE_SUB(NOW(), INTERVAL ? SECOND)`,
			username, int(lockoutWindow.Seconds()),
		).Scan(&count)
		if count == lockoutFailures {
			// Resolve user_id (best-effort: silent on error)
			var userID uint64
			_ = s.db.QueryRowContext(ctx,
				`SELECT id FROM users WHERE username = ?`, username,
			).Scan(&userID)
			if userID > 0 {
				until := time.Now().UTC().Add(lockoutDuration)
				go s.onLockout(context.Background(), userID, until)
			}
		}
	}
	return nil
}

func validatePassword(pw string) error {
	if len(pw) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	var hasUpper, hasDigit bool
	for _, c := range pw {
		if c >= 'A' && c <= 'Z' {
			hasUpper = true
		}
		if c >= '0' && c <= '9' {
			hasDigit = true
		}
	}
	if !hasUpper {
		return errors.New("password must contain at least one uppercase letter")
	}
	if !hasDigit {
		return errors.New("password must contain at least one digit")
	}
	return nil
}

func isDuplicate(err error) bool {
	return strings.Contains(err.Error(), "Duplicate entry") ||
		strings.Contains(err.Error(), "duplicate key")
}
