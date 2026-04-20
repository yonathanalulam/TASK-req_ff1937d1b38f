package config

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
)

// Config holds all runtime configuration sourced from environment variables.
type Config struct {
	// Application
	AppEnv string
	Port   string

	// Database
	DBHost     string
	DBPort     string
	DBName     string
	DBUser     string
	DBPassword string

	// Security
	FieldEncryptionKey  string // 32-byte hex-encoded AES-256 key
	SessionCookieDomain string

	// TLS (optional — empty disables TLS)
	TLSCertFile string
	TLSKeyFile  string
}

// placeholderFieldKeys is the set of well-known "example" values that must
// never run in production. The leading entry is the literal value shipped in
// .env.example; additional entries catch other common dummies (all-zeros,
// sequential nibbles) that developers sometimes reach for.
var placeholderFieldKeys = map[string]string{
	"1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef": "the .env.example placeholder",
	"0000000000000000000000000000000000000000000000000000000000000000": "all-zero dev key",
	"deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef": "dead-beef dev key",
}

// Load reads configuration from environment variables and validates required fields.
func Load() (*Config, error) {
	cfg := &Config{
		AppEnv:              getEnv("APP_ENV", "development"),
		Port:                getEnv("PORT", "8080"),
		DBHost:              getEnv("DB_HOST", "localhost"),
		DBPort:              getEnv("DB_PORT", "3306"),
		DBName:              mustEnv("DB_NAME"),
		DBUser:              mustEnv("DB_USER"),
		DBPassword:          mustEnv("DB_PASSWORD"),
		FieldEncryptionKey:  getEnv("FIELD_ENCRYPTION_KEY", ""),
		SessionCookieDomain: getEnv("SESSION_COOKIE_DOMAIN", "localhost"),
		TLSCertFile:         getEnv("TLS_CERT_FILE", ""),
		TLSKeyFile:          getEnv("TLS_KEY_FILE", ""),
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	var errs []error

	if c.DBName == "" {
		errs = append(errs, errors.New("DB_NAME is required"))
	}
	if c.DBUser == "" {
		errs = append(errs, errors.New("DB_USER is required"))
	}
	if c.DBPassword == "" {
		errs = append(errs, errors.New("DB_PASSWORD is required"))
	}

	keyErr := c.validateEncryptionKey()
	if keyErr != nil {
		errs = append(errs, keyErr)
	}

	if tlsErr := c.validateTLS(); tlsErr != nil {
		errs = append(errs, tlsErr)
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed: %v", errs)
	}
	return nil
}

// validateTLS enforces the "TLS everywhere" policy: both TLS_CERT_FILE and
// TLS_KEY_FILE must be set and point at a readable file on disk, in every
// environment (development, test, production). There is no HTTP fallback —
// the prompt explicitly requires that all transport, even on a local network,
// be secured with TLS.
//
// The file-on-disk check is delegated to main.go's startup path so unit tests
// that construct a *Config directly (or via Load with env-supplied paths that
// exist only in the test harness) don't have to materialise real cert files.
func (c *Config) validateTLS() error {
	if c.TLSCertFile == "" || c.TLSKeyFile == "" {
		return errors.New(
			"TLS_CERT_FILE and TLS_KEY_FILE are required in all environments " +
				"(TLS is mandatory — no HTTP fallback). Run ./scripts/gen-tls-cert.sh to generate a dev pair")
	}
	return nil
}

// validateEncryptionKey enforces the rules for FIELD_ENCRYPTION_KEY:
//   - test mode: empty allowed (the crypto helpers bypass AES when key == "")
//   - production: must be present, 64 hex chars (32 bytes), AND not a known
//     placeholder. Starting production with the example key would silently
//     render all "encrypted" PII trivially decryptable by anyone with the repo.
//   - development: must be present OR empty; a placeholder is tolerated but
//     emits a loud warning so the condition doesn't stay silent.
func (c *Config) validateEncryptionKey() error {
	key := c.FieldEncryptionKey
	isPlaceholder, placeholderLabel := c.isPlaceholderKey()

	if c.AppEnv == "test" {
		// Empty is the convention for tests; bypass all further checks.
		if key == "" {
			return nil
		}
	}

	if key == "" {
		return errors.New("FIELD_ENCRYPTION_KEY is required")
	}

	// Must be a 32-byte AES-256 key, hex-encoded (64 chars).
	decoded, err := hex.DecodeString(key)
	if err != nil {
		return fmt.Errorf("FIELD_ENCRYPTION_KEY must be hex-encoded: %w", err)
	}
	if len(decoded) != 32 {
		return fmt.Errorf("FIELD_ENCRYPTION_KEY must be exactly 32 bytes (64 hex chars), got %d bytes", len(decoded))
	}

	if isPlaceholder {
		if c.IsProduction() {
			return fmt.Errorf(
				"FIELD_ENCRYPTION_KEY is set to %s — refusing to start in production. "+
					"Generate a fresh key with `./scripts/gen-key.sh` (or `openssl rand -hex 32`) "+
					"and set it via FIELD_ENCRYPTION_KEY before deploying.",
				placeholderLabel,
			)
		}
		log.Printf("[SECURITY WARNING] FIELD_ENCRYPTION_KEY is %s — do NOT use this value in production (APP_ENV=%s)",
			placeholderLabel, c.AppEnv)
	}

	return nil
}

// isPlaceholderKey reports whether the configured key matches a known dummy
// value. Comparison is case-insensitive because hex is case-insensitive.
func (c *Config) isPlaceholderKey() (bool, string) {
	norm := strings.ToLower(c.FieldEncryptionKey)
	if label, ok := placeholderFieldKeys[norm]; ok {
		return true, label
	}
	return false, ""
}

// DSN returns the MySQL data source name string.
func (c *Config) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&multiStatements=true&charset=utf8mb4",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
}

// IsProduction returns true when running in production mode.
func (c *Config) IsProduction() bool {
	return c.AppEnv == "production"
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mustEnv(key string) string {
	return os.Getenv(key)
}
