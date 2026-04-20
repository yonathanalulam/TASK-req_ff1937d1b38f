package address

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/eagle-point/service-portal/internal/crypto"
	"github.com/eagle-point/service-portal/internal/models"
)

// ErrNotFound is returned when an address does not exist or belongs to another user.
var ErrNotFound = errors.New("address not found")

var zipRegex = regexp.MustCompile(`^\d{5}(-\d{4})?$`)

// ValidateZip returns true when zip matches the US format ^\d{5}(-\d{4})?$
func ValidateZip(zip string) bool {
	return zipRegex.MatchString(zip)
}

// Service provides address-book CRUD with AES field encryption.
type Service struct {
	db     *sql.DB
	encKey string
}

// NewService creates a Service.
func NewService(db *sql.DB, encKey string) *Service {
	return &Service{db: db, encKey: encKey}
}

// CreateInput carries validated address creation fields.
type CreateInput struct {
	Label        string
	AddressLine1 string
	AddressLine2 string
	City         string
	State        string
	Zip          string
}

// UpdateInput carries validated address update fields.
type UpdateInput = CreateInput

// List returns all addresses for a user, ordered by default-first then created_at.
func (s *Service) List(ctx context.Context, userID uint64) ([]*models.Address, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, label, address_line1_encrypted, address_line2_encrypted,
		        city, state, zip, is_default, created_at, updated_at
		 FROM addresses
		 WHERE user_id = ?
		 ORDER BY is_default DESC, created_at ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("address.List: %w", err)
	}
	defer rows.Close()

	var addrs []*models.Address
	for rows.Next() {
		a, err := s.scanAddress(rows)
		if err != nil {
			return nil, fmt.Errorf("address.List: scan: %w", err)
		}
		addrs = append(addrs, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("address.List: rows: %w", err)
	}
	if addrs == nil {
		addrs = []*models.Address{}
	}
	return addrs, nil
}

// Create adds a new address for the user. The first address is automatically
// set as default.
func (s *Service) Create(ctx context.Context, userID uint64, in CreateInput) (*models.Address, error) {
	if err := validateInput(in); err != nil {
		return nil, err
	}

	line1Enc, err := encryptField(in.AddressLine1, s.encKey)
	if err != nil {
		return nil, fmt.Errorf("address.Create: encrypt line1: %w", err)
	}
	var line2Enc []byte
	if in.AddressLine2 != "" {
		line2Enc, err = encryptField(in.AddressLine2, s.encKey)
		if err != nil {
			return nil, fmt.Errorf("address.Create: encrypt line2: %w", err)
		}
	}

	// First address for this user auto-becomes default.
	var existingCount int
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM addresses WHERE user_id = ?`, userID).Scan(&existingCount)
	isDefault := existingCount == 0

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO addresses
		   (user_id, label, address_line1_encrypted, address_line2_encrypted,
		    city, state, zip, is_default)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		userID,
		strings.TrimSpace(in.Label),
		line1Enc, line2Enc,
		strings.TrimSpace(in.City),
		strings.ToUpper(strings.TrimSpace(in.State)),
		in.Zip,
		isDefault,
	)
	if err != nil {
		return nil, fmt.Errorf("address.Create: insert: %w", err)
	}

	id, _ := res.LastInsertId()
	return s.getByID(ctx, uint64(id), userID)
}

// Update replaces address fields for an existing address owned by userID.
func (s *Service) Update(ctx context.Context, userID, addrID uint64, in UpdateInput) (*models.Address, error) {
	if err := validateInput(in); err != nil {
		return nil, err
	}

	line1Enc, err := encryptField(in.AddressLine1, s.encKey)
	if err != nil {
		return nil, fmt.Errorf("address.Update: encrypt line1: %w", err)
	}
	var line2Enc []byte
	if in.AddressLine2 != "" {
		line2Enc, err = encryptField(in.AddressLine2, s.encKey)
		if err != nil {
			return nil, fmt.Errorf("address.Update: encrypt line2: %w", err)
		}
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE addresses
		 SET label=?, address_line1_encrypted=?, address_line2_encrypted=?,
		     city=?, state=?, zip=?
		 WHERE id = ? AND user_id = ?`,
		strings.TrimSpace(in.Label),
		line1Enc, line2Enc,
		strings.TrimSpace(in.City),
		strings.ToUpper(strings.TrimSpace(in.State)),
		in.Zip,
		addrID, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("address.Update: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.getByID(ctx, addrID, userID)
}

// Delete removes an address owned by userID.
func (s *Service) Delete(ctx context.Context, userID, addrID uint64) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM addresses WHERE id = ? AND user_id = ?`, addrID, userID)
	if err != nil {
		return fmt.Errorf("address.Delete: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetDefault promotes addrID to the default and clears the previous default.
func (s *Service) SetDefault(ctx context.Context, userID, addrID uint64) (*models.Address, error) {
	// Verify ownership first.
	if _, err := s.getByID(ctx, addrID, userID); err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("address.SetDefault: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Clear all defaults for this user.
	if _, err := tx.ExecContext(ctx,
		`UPDATE addresses SET is_default = 0 WHERE user_id = ?`, userID); err != nil {
		return nil, fmt.Errorf("address.SetDefault: clear defaults: %w", err)
	}

	// Set the new default.
	if _, err := tx.ExecContext(ctx,
		`UPDATE addresses SET is_default = 1 WHERE id = ? AND user_id = ?`,
		addrID, userID); err != nil {
		return nil, fmt.Errorf("address.SetDefault: set default: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("address.SetDefault: commit: %w", err)
	}

	return s.getByID(ctx, addrID, userID)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

type rowScanner interface {
	Scan(dest ...any) error
}

func (s *Service) scanAddress(rs rowScanner) (*models.Address, error) {
	a := &models.Address{}
	var line1Enc, line2Enc []byte
	err := rs.Scan(
		&a.ID, &a.UserID, &a.Label,
		&line1Enc, &line2Enc,
		&a.City, &a.State, &a.Zip, &a.IsDefault,
		&a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if len(line1Enc) > 0 {
		a.AddressLine1, _ = decryptField(line1Enc, s.encKey)
	}
	if len(line2Enc) > 0 {
		a.AddressLine2, _ = decryptField(line2Enc, s.encKey)
	}
	return a, nil
}

func (s *Service) getByID(ctx context.Context, addrID, userID uint64) (*models.Address, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, label, address_line1_encrypted, address_line2_encrypted,
		        city, state, zip, is_default, created_at, updated_at
		 FROM addresses WHERE id = ? AND user_id = ?`,
		addrID, userID,
	)
	a, err := s.scanAddress(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

func validateInput(in CreateInput) error {
	if strings.TrimSpace(in.AddressLine1) == "" {
		return errors.New("address_line1 is required")
	}
	if strings.TrimSpace(in.City) == "" {
		return errors.New("city is required")
	}
	if strings.TrimSpace(in.State) == "" {
		return errors.New("state is required")
	}
	if !ValidateZip(in.Zip) {
		return fmt.Errorf("zip %q is not a valid US ZIP code (expected NNNNN or NNNNN-NNNN)", in.Zip)
	}
	if in.Label == "" {
		in.Label = "Home"
	}
	return nil
}

func encryptField(value, encKey string) ([]byte, error) {
	if encKey == "" {
		return []byte(value), nil
	}
	return crypto.EncryptString(value, encKey)
}

func decryptField(data []byte, encKey string) (string, error) {
	if encKey == "" || len(data) == 0 {
		return string(data), nil
	}
	return crypto.DecryptString(data, encKey)
}
