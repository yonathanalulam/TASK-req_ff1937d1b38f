package hmacadmin

import (
	"context"
	"database/sql"
	"fmt"
	"log"
)

// DevBootstrapKeyID is the well-known key_id minted by EnsureDevKey when the
// hmac_keys table is empty in a development environment. It is NOT created in
// production or test mode.
const DevBootstrapKeyID = "dev-key-001"

// EnsureDevKey creates a single AES-encrypted HMAC key when the table is
// empty. Intended to be called from startup code in development environments
// so `docker-compose up` yields a working signed-request flow without forcing
// the admin to hit the create endpoint first.
//
// Behaviour:
//   - If any row exists in hmac_keys: no-op, returns nil.
//   - Otherwise: creates DevBootstrapKeyID with a fresh random secret,
//     encrypted under encKey, and logs the plaintext secret to stdout so the
//     developer can copy it into their client config.
//
// Production callers must NOT invoke this — admins should provision keys via
// the admin API so the plaintext never lands in server logs.
func EnsureDevKey(ctx context.Context, db *sql.DB, encKey string) error {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM hmac_keys`).Scan(&count); err != nil {
		return fmt.Errorf("hmacadmin.EnsureDevKey: count: %w", err)
	}
	if count > 0 {
		return nil
	}

	svc := NewService(db, encKey)
	reveal, err := svc.Create(ctx, DevBootstrapKeyID)
	if err != nil {
		return fmt.Errorf("hmacadmin.EnsureDevKey: create: %w", err)
	}

	// Logged with a distinctive prefix so developers can grep for it in
	// `docker-compose logs backend`. This is intentionally loud because the
	// secret cannot be retrieved later.
	log.Printf("[hmacadmin] bootstrapped dev HMAC key: key_id=%q secret=%s (DEVELOPMENT ONLY — rotate or revoke before production)",
		reveal.KeyID, reveal.Secret)

	return nil
}
