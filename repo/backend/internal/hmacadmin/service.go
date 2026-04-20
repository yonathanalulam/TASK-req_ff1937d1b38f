// Package hmacadmin manages the lifecycle of HMAC signing keys used by
// internal clients (ingest workers, lakehouse jobs) to authenticate against
// /api/v1/internal/* routes.
//
// Keys are stored in the hmac_keys table. Each row has a human-readable key_id
// that clients send in the X-Key-ID request header and an AES-256-GCM
// encrypted secret used to verify the HMAC-SHA256 signature of the request.
//
// Lifecycle operations:
//
//   - Create  — mint a brand new key_id with a server-generated secret. The
//     plaintext secret is returned to the caller EXACTLY ONCE; after the
//     response it cannot be retrieved again.
//   - Rotate  — replace the encrypted secret on an existing key_id with a
//     freshly generated one. The row is updated in place (UNIQUE constraint
//     on key_id prevents keeping old versions), so rotation is a hard
//     cut-over: clients holding the old secret immediately start receiving
//     401s until they pick up the new one.
//   - Revoke  — flip is_active=0 so the verifier stops accepting the key
//     without deleting the row (preserves audit history).
//   - List    — enumerate metadata (never secrets).
//
// The package is deliberately narrow: it does not handle client distribution,
// grace-period overlaps, or asymmetric schemes. A future enhancement could
// relax the UNIQUE(key_id) constraint to support dual-accept windows.
package hmacadmin

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"time"

	appCrypto "github.com/eagle-point/service-portal/internal/crypto"
)

// SecretByteLength is the size of freshly generated HMAC secrets. 32 bytes
// (256 bits) matches the SHA-256 block size and exceeds the NIST SP 800-107
// minimum for HMAC-SHA-256.
const SecretByteLength = 32

// Sentinel errors. Handlers translate these to HTTP status codes.
var (
	ErrKeyIDRequired = errors.New("hmacadmin: key_id is required")
	ErrKeyIDInvalid  = errors.New("hmacadmin: key_id contains invalid characters")
	ErrKeyIDExists   = errors.New("hmacadmin: key_id already exists")
	ErrKeyNotFound   = errors.New("hmacadmin: key not found")
)

// keyIDPattern restricts key_id to a safe subset: 1–64 chars, letters, digits,
// dash, underscore, dot. This prevents whitespace/control chars that would
// break header parsing and keeps key_ids readable in logs.
var keyIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// KeyInfo is the metadata projection returned to admins. It deliberately
// omits the encrypted or plaintext secret.
type KeyInfo struct {
	ID        uint64     `json:"id"`
	KeyID     string     `json:"key_id"`
	IsActive  bool       `json:"is_active"`
	CreatedAt time.Time  `json:"created_at"`
	RotatedAt *time.Time `json:"rotated_at,omitempty"`
}

// SecretReveal is the one-shot response returned when a secret is generated
// (either by Create or Rotate). The plaintext field is not persisted anywhere
// and cannot be re-fetched — callers must copy it to the client immediately.
type SecretReveal struct {
	KeyInfo
	Secret string `json:"secret"` // hex-encoded SecretByteLength random bytes
}

// Service owns all HMAC key CRUD. Construct once and share; methods are safe
// for concurrent use (database/sql handles synchronization).
type Service struct {
	db     *sql.DB
	encKey string // FIELD_ENCRYPTION_KEY; hex-encoded AES-256 key
}

// NewService constructs a Service. The encryption key must be valid for the
// AES helpers (32 bytes, hex-encoded) — validation is deferred to the first
// encrypt/decrypt call so that configuration errors surface there rather than
// at startup.
func NewService(db *sql.DB, encKey string) *Service {
	return &Service{db: db, encKey: encKey}
}

// List returns every key's metadata, newest first.
func (s *Service) List(ctx context.Context) ([]KeyInfo, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, key_id, is_active, created_at, rotated_at
		   FROM hmac_keys
		  ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("hmacadmin.List: %w", err)
	}
	defer rows.Close()

	out := []KeyInfo{}
	for rows.Next() {
		var k KeyInfo
		var rotated sql.NullTime
		var isActive int
		if err := rows.Scan(&k.ID, &k.KeyID, &isActive, &k.CreatedAt, &rotated); err != nil {
			return nil, fmt.Errorf("hmacadmin.List: scan: %w", err)
		}
		k.IsActive = isActive == 1
		if rotated.Valid {
			t := rotated.Time
			k.RotatedAt = &t
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// Create mints a new key_id with a random secret. Fails if key_id is missing,
// malformed, or already exists. Returns the plaintext secret exactly once.
func (s *Service) Create(ctx context.Context, keyID string) (*SecretReveal, error) {
	if keyID == "" {
		return nil, ErrKeyIDRequired
	}
	if !keyIDPattern.MatchString(keyID) {
		return nil, ErrKeyIDInvalid
	}

	// Early uniqueness check for a clean error; the INSERT below would also
	// fail via the UNIQUE constraint but would return a driver-specific error.
	var existing int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM hmac_keys WHERE key_id = ?`, keyID).Scan(&existing)
	if err == nil {
		return nil, ErrKeyIDExists
	}
	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("hmacadmin.Create: lookup: %w", err)
	}

	secretBytes, secretHex, err := generateSecret()
	if err != nil {
		return nil, err
	}
	encrypted, err := appCrypto.Encrypt(secretBytes, s.encKey)
	if err != nil {
		return nil, fmt.Errorf("hmacadmin.Create: encrypt: %w", err)
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO hmac_keys (key_id, secret_encrypted, is_active)
		 VALUES (?, ?, 1)`, keyID, encrypted)
	if err != nil {
		return nil, fmt.Errorf("hmacadmin.Create: insert: %w", err)
	}
	id, _ := res.LastInsertId()

	info, err := s.getByID(ctx, uint64(id))
	if err != nil {
		return nil, err
	}
	return &SecretReveal{KeyInfo: *info, Secret: secretHex}, nil
}

// Rotate generates a new secret for an existing key_id and overwrites the
// encrypted column. The old secret becomes immediately unusable (hard
// cutover). Returns the new plaintext secret exactly once. Reactivates the
// row if it was previously revoked — rotation implies "this key is in use
// again".
func (s *Service) Rotate(ctx context.Context, keyID string) (*SecretReveal, error) {
	if keyID == "" {
		return nil, ErrKeyIDRequired
	}

	secretBytes, secretHex, err := generateSecret()
	if err != nil {
		return nil, err
	}
	encrypted, err := appCrypto.Encrypt(secretBytes, s.encKey)
	if err != nil {
		return nil, fmt.Errorf("hmacadmin.Rotate: encrypt: %w", err)
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE hmac_keys
		    SET secret_encrypted = ?,
		        rotated_at       = ?,
		        is_active        = 1
		  WHERE key_id = ?`,
		encrypted, time.Now().UTC(), keyID)
	if err != nil {
		return nil, fmt.Errorf("hmacadmin.Rotate: update: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return nil, ErrKeyNotFound
	}

	info, err := s.getByKeyID(ctx, keyID)
	if err != nil {
		return nil, err
	}
	return &SecretReveal{KeyInfo: *info, Secret: secretHex}, nil
}

// Revoke marks a key inactive. The row is kept to preserve audit trails
// (created_at, rotated_at) and to prevent the same key_id from being reused
// accidentally.
func (s *Service) Revoke(ctx context.Context, id uint64) (*KeyInfo, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE hmac_keys SET is_active = 0 WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("hmacadmin.Revoke: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return nil, ErrKeyNotFound
	}
	return s.getByID(ctx, id)
}

// ─── internals ───────────────────────────────────────────────────────────────

func (s *Service) getByID(ctx context.Context, id uint64) (*KeyInfo, error) {
	return s.selectOne(ctx,
		`SELECT id, key_id, is_active, created_at, rotated_at FROM hmac_keys WHERE id = ?`, id)
}

func (s *Service) getByKeyID(ctx context.Context, keyID string) (*KeyInfo, error) {
	return s.selectOne(ctx,
		`SELECT id, key_id, is_active, created_at, rotated_at FROM hmac_keys WHERE key_id = ?`, keyID)
}

func (s *Service) selectOne(ctx context.Context, query string, arg any) (*KeyInfo, error) {
	var k KeyInfo
	var rotated sql.NullTime
	var isActive int
	err := s.db.QueryRowContext(ctx, query, arg).
		Scan(&k.ID, &k.KeyID, &isActive, &k.CreatedAt, &rotated)
	if err == sql.ErrNoRows {
		return nil, ErrKeyNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("hmacadmin: select: %w", err)
	}
	k.IsActive = isActive == 1
	if rotated.Valid {
		t := rotated.Time
		k.RotatedAt = &t
	}
	return &k, nil
}

// generateSecret produces SecretByteLength random bytes and their hex encoding.
// Returning both avoids re-encoding overhead on the happy path.
func generateSecret() ([]byte, string, error) {
	b := make([]byte, SecretByteLength)
	if _, err := rand.Read(b); err != nil {
		return nil, "", fmt.Errorf("hmacadmin: generate secret: %w", err)
	}
	return b, hex.EncodeToString(b), nil
}
