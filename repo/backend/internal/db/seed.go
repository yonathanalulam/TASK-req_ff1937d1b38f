package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// SeedUserPassword is the plaintext password applied to every seeded user.
// Centralised here so the seed and the README stay in sync.
const SeedUserPassword = "password"

// seedUsernames lists the canonical seed accounts. After every seed run their
// password_hash is rewritten with a freshly bcrypted hash, and their account
// state is reset to a known-good baseline (active, not deleted, not frozen,
// no stale failed-login attempts). This makes seed runs idempotent even if
// earlier test runs deactivated, deleted, or locked out one of these users.
var seedUsernames = []string{
	"regular_user",
	"service_agent",
	"moderator",
	"admin",
	"data_operator",
}

// Seed executes all SQL files found in seedDir in alphabetical order, then
// resets every canonical seed user to a known-good baseline so the published
// dev credentials always work, even after destructive tests.
func Seed(db *sql.DB, seedDir string) error {
	pattern := filepath.Join(seedDir, "*.sql")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("seed: glob %s: %w", pattern, err)
	}

	if len(files) == 0 {
		return fmt.Errorf("seed: no SQL files found in %s", seedDir)
	}

	sort.Strings(files)

	for _, f := range files {
		if err := executeSQLFile(db, f); err != nil {
			return fmt.Errorf("seed: execute %s: %w", filepath.Base(f), err)
		}
	}

	if err := resetSeedUsers(db); err != nil {
		return fmt.Errorf("seed: reset users: %w", err)
	}

	return nil
}

// resetSeedUsers brings every seed account back to a working baseline:
//   - password_hash = freshly-computed bcrypt of SeedUserPassword
//   - is_active = 1, is_deleted = 0
//   - posting_freeze_until = NULL (clears any moderation freeze)
//   - clears the user's recent failed-login attempts so lockouts don't carry over
//
// Logs each user it touched so `docker-compose logs seed` makes it obvious
// the reset actually ran.
func resetSeedUsers(db *sql.DB) error {
	for _, u := range seedUsernames {
		hash, err := bcrypt.GenerateFromPassword([]byte(SeedUserPassword), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("bcrypt: %w", err)
		}
		hashStr := string(hash)

		res, err := db.Exec(
			`UPDATE users
			 SET password_hash       = ?,
			     is_active           = 1,
			     is_deleted          = 0,
			     posting_freeze_until = NULL,
			     anonymized_at       = NULL,
			     avatar_url          = COALESCE(avatar_url, ''),
			     bio                 = COALESCE(bio, '')
			 WHERE username = ?`,
			hashStr, u,
		)
		if err != nil {
			return fmt.Errorf("update %s: %w", u, err)
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			log.Printf("seed: WARNING: user %q not found — skipping reset", u)
			continue
		}

		// Wipe any stale failed-login rows so a previously-locked-out account
		// can sign in immediately after re-seeding.
		if _, err := db.Exec(
			`DELETE FROM login_attempts WHERE username = ?`, u,
		); err != nil {
			// Not fatal — just log
			log.Printf("seed: could not clear login_attempts for %q: %v", u, err)
		}

		log.Printf("seed: reset %q (password=%q, active=1, freeze=NULL)", u, SeedUserPassword)
	}
	return nil
}

func executeSQLFile(db *sql.DB, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	content := strings.TrimSpace(string(raw))
	if content == "" {
		return nil
	}

	if _, err := db.Exec(content); err != nil {
		return err
	}

	return nil
}
