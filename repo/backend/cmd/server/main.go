package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"

	"github.com/eagle-point/service-portal/internal/config"
	appdb "github.com/eagle-point/service-portal/internal/db"
	"github.com/eagle-point/service-portal/internal/router"
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	mode := "server"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}

	switch mode {
	case "migrate":
		runMigrate()
	case "seed":
		runSeed()
	case "checkpw":
		runCheckPw()
	case "resetpw":
		runResetPw()
	default:
		runServer()
	}
}

// ── Diagnostic: verify a stored hash + password without going through HTTP ──
//
// Usage:
//   docker-compose exec backend /server checkpw admin password
//
// Reports the exact gate that's failing (user-not-found, deactivated, deleted,
// hash-mismatch). Only intended for development environments.
func runCheckPw() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "usage: server checkpw <username> <password>")
		os.Exit(2)
	}
	username := os.Args[2]
	password := os.Args[3]

	cfg := mustLoadConfig()
	database := mustOpenDB(cfg)
	defer database.Close()

	var (
		id        uint64
		hash      string
		isActive  bool
		isDeleted bool
		freeze    sql.NullTime
	)
	err := database.QueryRow(
		`SELECT id, password_hash, is_active, is_deleted, posting_freeze_until
		 FROM users WHERE username = ?`, username,
	).Scan(&id, &hash, &isActive, &isDeleted, &freeze)

	switch {
	case err == sql.ErrNoRows:
		fmt.Printf("FAIL: user %q does NOT exist in the users table\n", username)
		os.Exit(1)
	case err != nil:
		fmt.Printf("FAIL: db query error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("user %q found:\n", username)
	fmt.Printf("  id                   = %d\n", id)
	fmt.Printf("  is_active            = %v\n", isActive)
	fmt.Printf("  is_deleted           = %v\n", isDeleted)
	fmt.Printf("  posting_freeze_until = %v\n", freeze)
	fmt.Printf("  hash_prefix          = %s...  (len=%d)\n", safePrefix(hash, 15), len(hash))

	if isDeleted {
		fmt.Println("FAIL: getUserByUsername filters WHERE is_deleted=0 → user invisible to login")
		os.Exit(1)
	}
	if !isActive {
		fmt.Println("FAIL: is_active=0 → login returns invalid_credentials")
		os.Exit(1)
	}

	if cmpErr := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); cmpErr != nil {
		fmt.Printf("FAIL: bcrypt mismatch: %v\n", cmpErr)
		fmt.Println("\nThe stored hash does NOT verify against the supplied password.")
		fmt.Println("Fix it with:  docker-compose exec backend /server resetpw " + username + " " + password)
		os.Exit(1)
	}
	fmt.Println("OK: bcrypt verifies — login should succeed for this user.")
}

// ── Diagnostic: forcibly reset a user's password from the CLI ──
//
// Usage:
//   docker-compose exec backend /server resetpw admin password
func runResetPw() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "usage: server resetpw <username> <new_password>")
		os.Exit(2)
	}
	username := os.Args[2]
	password := os.Args[3]

	cfg := mustLoadConfig()
	database := mustOpenDB(cfg)
	defer database.Close()

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		fmt.Printf("FAIL: bcrypt: %v\n", err)
		os.Exit(1)
	}
	res, err := database.Exec(
		`UPDATE users
		 SET password_hash = ?, is_active = 1, is_deleted = 0,
		     posting_freeze_until = NULL, anonymized_at = NULL
		 WHERE username = ?`,
		string(hash), username,
	)
	if err != nil {
		fmt.Printf("FAIL: update: %v\n", err)
		os.Exit(1)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		fmt.Printf("FAIL: no user named %q\n", username)
		os.Exit(1)
	}
	_, _ = database.Exec(`DELETE FROM login_attempts WHERE username = ?`, username)
	fmt.Printf("OK: %q password reset (active=1, deleted=0, freeze=NULL, attempts cleared)\n", username)
}

func safePrefix(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[:n]
}

// ── Server ─────────────────────────────────────────────────────────────────────

func runServer() {
	cfg := mustLoadConfig()
	database := mustOpenDB(cfg)
	defer database.Close()

	r := router.New(cfg, database)

	addr := fmt.Sprintf(":%s", cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// TLS policy: MANDATORY in every environment. There is no plain-HTTP path
	// — the Prompt requires "all transport is secured with TLS even on a local
	// network", so a missing or unreadable cert/key is a fatal startup error.
	// config.Load already validates the env vars are present; here we confirm
	// the files on disk are readable before we start listening, giving the
	// operator a clear actionable error instead of a mid-handshake failure.
	tlsCert := cfg.TLSCertFile
	tlsKey := cfg.TLSKeyFile
	if tlsCert == "" || tlsKey == "" {
		log.Fatal().Msg(
			"TLS is required in all environments: set TLS_CERT_FILE and TLS_KEY_FILE in your .env. " +
				"Run ./scripts/gen-tls-cert.sh to produce a self-signed pair for local development/testing.")
	}
	if _, err := os.Stat(tlsCert); err != nil {
		log.Fatal().Err(err).Str("path", tlsCert).Msg(
			"TLS_CERT_FILE not readable — generate one with ./scripts/gen-tls-cert.sh")
	}
	if _, err := os.Stat(tlsKey); err != nil {
		log.Fatal().Err(err).Str("path", tlsKey).Msg(
			"TLS_KEY_FILE not readable — generate one with ./scripts/gen-tls-cert.sh")
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Info().Str("addr", addr).Str("cert", tlsCert).Msg("starting server (TLS)")
		if err := srv.ListenAndServeTLS(tlsCert, tlsKey); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server error")
		}
	}()

	<-quit
	log.Info().Msg("shutting down…")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("forced shutdown")
	}
	log.Info().Msg("server stopped")
}

// ── Migrate ────────────────────────────────────────────────────────────────────

func runMigrate() {
	cfg := mustLoadConfig()
	database := mustOpenDB(cfg)
	defer database.Close()

	log.Info().Msg("running migrations…")
	if err := appdb.Migrate(database); err != nil {
		log.Fatal().Err(err).Msg("migration failed")
	}
	log.Info().Msg("migrations complete")
}

// ── Seed ───────────────────────────────────────────────────────────────────────

func runSeed() {
	cfg := mustLoadConfig()
	database := mustOpenDB(cfg)
	defer database.Close()

	seedDir := getEnvOrDefault("SEED_DIR", "./seeds")
	log.Info().Str("dir", seedDir).Msg("seeding database…")
	if err := appdb.Seed(database, seedDir); err != nil {
		log.Fatal().Err(err).Msg("seed failed")
	}
	log.Info().Msg("seed complete")
}

// ── Helpers ────────────────────────────────────────────────────────────────────

func mustLoadConfig() *config.Config {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("config error")
	}
	return cfg
}

func mustOpenDB(cfg *config.Config) *sql.DB {
	database, err := appdb.Open(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("db connection failed")
	}
	log.Info().Str("host", cfg.DBHost).Str("db", cfg.DBName).Msg("database connected")
	return database
}

func getEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
