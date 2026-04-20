// Package testutil provides helpers for integration tests that need a real database.
// Tests call DBOrSkip(t) — if the required env vars are absent the test is skipped
// rather than failed, so unit-only runs remain clean.
package testutil

import (
	"database/sql"
	"os"
	"testing"

	"github.com/eagle-point/service-portal/internal/config"
	appdb "github.com/eagle-point/service-portal/internal/db"
)

// DBOrSkip opens a connection to the test database.
// If DB_HOST / DB_NAME are not set it calls t.Skip() so the test is omitted
// in unit-only runs (e.g. local `go test ./...` without Docker).
func DBOrSkip(t *testing.T) *sql.DB {
	t.Helper()

	if os.Getenv("DB_HOST") == "" || os.Getenv("DB_NAME") == "" {
		t.Skip("integration test requires DB_HOST and DB_NAME env vars (run inside Docker)")
	}

	// Force test mode so FIELD_ENCRYPTION_KEY is not required
	os.Setenv("APP_ENV", "test")

	// TLS paths are required by config.Load() in every environment (no HTTP
	// fallback). The Go test runner uses httptest.NewServer, which does not
	// actually read these files — any non-empty string satisfies the config
	// validator and lets the integration tests reach the DB. Set only if the
	// operator hasn't already provided real paths.
	if os.Getenv("TLS_CERT_FILE") == "" {
		os.Setenv("TLS_CERT_FILE", "/dev/null/test-cert.pem")
	}
	if os.Getenv("TLS_KEY_FILE") == "" {
		os.Setenv("TLS_KEY_FILE", "/dev/null/test-key.pem")
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("testutil.DBOrSkip: config: %v", err)
	}

	db, err := appdb.Open(cfg)
	if err != nil {
		t.Fatalf("testutil.DBOrSkip: open: %v", err)
	}

	if err := appdb.Migrate(db); err != nil {
		t.Fatalf("testutil.DBOrSkip: migrate: %v", err)
	}

	// Register cleanup so each test leaves a clean DB state
	t.Cleanup(func() { db.Close() })

	return db
}

// TruncateTables removes all rows from the given tables.
// Call this at the start of integration tests to ensure isolation.
func TruncateTables(t *testing.T, db *sql.DB, tables ...string) {
	t.Helper()
	_, _ = db.Exec("SET FOREIGN_KEY_CHECKS = 0")
	for _, table := range tables {
		if _, err := db.Exec("TRUNCATE TABLE `" + table + "`"); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
	_, _ = db.Exec("SET FOREIGN_KEY_CHECKS = 1")
}
