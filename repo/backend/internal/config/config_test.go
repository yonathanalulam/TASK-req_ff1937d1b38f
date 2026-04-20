package config_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eagle-point/service-portal/internal/config"
)

// setBaseEnv sets the DB + TLS env vars every test needs and unsets the rest
// so cases start from a clean slate. TLS paths are required in every
// environment (no HTTP fallback), so the base env provides dummy values that
// satisfy Load()'s validation; tests asserting *missing* TLS clear them
// explicitly.
func setBaseEnv(t *testing.T, appEnv string) {
	t.Helper()
	os.Setenv("APP_ENV", appEnv)
	os.Setenv("DB_NAME", "testdb")
	os.Setenv("DB_USER", "testuser")
	os.Setenv("DB_PASSWORD", "testpass")
	os.Setenv("TLS_CERT_FILE", "/dev/null/cert.pem")
	os.Setenv("TLS_KEY_FILE", "/dev/null/key.pem")
	os.Unsetenv("FIELD_ENCRYPTION_KEY")
	t.Cleanup(func() {
		os.Unsetenv("APP_ENV")
		os.Unsetenv("DB_NAME")
		os.Unsetenv("DB_USER")
		os.Unsetenv("DB_PASSWORD")
		os.Unsetenv("TLS_CERT_FILE")
		os.Unsetenv("TLS_KEY_FILE")
		os.Unsetenv("FIELD_ENCRYPTION_KEY")
	})
}

func TestLoad_RequiredFields(t *testing.T) {
	// Clear env
	os.Unsetenv("DB_NAME")
	os.Unsetenv("DB_USER")
	os.Unsetenv("DB_PASSWORD")
	os.Unsetenv("FIELD_ENCRYPTION_KEY")
	os.Unsetenv("TLS_CERT_FILE")
	os.Unsetenv("TLS_KEY_FILE")
	os.Setenv("APP_ENV", "test")
	t.Cleanup(func() { os.Unsetenv("APP_ENV") })

	_, err := config.Load()
	// In test mode the encryption key is optional, but DB fields AND TLS
	// paths are required everywhere — the error message should mention both.
	assert.Error(t, err)
}

func TestLoad_ValidConfig(t *testing.T) {
	setBaseEnv(t, "test")

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, "testdb", cfg.DBName)
	assert.Equal(t, "testuser", cfg.DBUser)
}

func TestConfig_DSN(t *testing.T) {
	setBaseEnv(t, "test")
	os.Setenv("DB_NAME", "mydb")
	os.Setenv("DB_USER", "user")
	os.Setenv("DB_PASSWORD", "pass")

	cfg, err := config.Load()
	require.NoError(t, err)

	dsn := cfg.DSN()
	assert.Contains(t, dsn, "user:pass@tcp(")
	assert.Contains(t, dsn, "/mydb")
}

// ─── FIELD_ENCRYPTION_KEY hardening ──────────────────────────────────────────

const (
	placeholderKey = "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	zerosKey       = "0000000000000000000000000000000000000000000000000000000000000000"
	validRealKey   = "a1b2c3d4e5f67890a1b2c3d4e5f67890a1b2c3d4e5f67890a1b2c3d4e5f67890"
)

func TestLoad_Production_RequiresKey(t *testing.T) {
	setBaseEnv(t, "production")
	// No FIELD_ENCRYPTION_KEY set

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "FIELD_ENCRYPTION_KEY is required")
}

func TestLoad_Production_RejectsEnvExamplePlaceholder(t *testing.T) {
	setBaseEnv(t, "production")
	os.Setenv("FIELD_ENCRYPTION_KEY", placeholderKey)

	_, err := config.Load()
	require.Error(t, err, "production must reject the .env.example placeholder")
	assert.Contains(t, err.Error(), "placeholder")
	assert.Contains(t, err.Error(), "refusing to start in production")
}

func TestLoad_Production_RejectsZeroKey(t *testing.T) {
	setBaseEnv(t, "production")
	os.Setenv("FIELD_ENCRYPTION_KEY", zerosKey)

	_, err := config.Load()
	require.Error(t, err, "production must reject the all-zero dev key")
	assert.Contains(t, strings.ToLower(err.Error()), "refusing to start in production")
}

func TestLoad_Production_RejectsPlaceholderCaseInsensitive(t *testing.T) {
	setBaseEnv(t, "production")
	// Same bytes, uppercased — hex is case-insensitive so this is the same key.
	os.Setenv("FIELD_ENCRYPTION_KEY", strings.ToUpper(placeholderKey))

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to start in production")
}

func TestLoad_Production_RejectsNonHexKey(t *testing.T) {
	setBaseEnv(t, "production")
	os.Setenv("FIELD_ENCRYPTION_KEY", "not-hex-at-all-not-hex-at-all-not-hex-at-all-not-hex-at-all-1234")

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hex-encoded")
}

func TestLoad_Production_RejectsWrongLengthKey(t *testing.T) {
	setBaseEnv(t, "production")
	// 16-byte key (AES-128) — not allowed, we require AES-256.
	os.Setenv("FIELD_ENCRYPTION_KEY", "a1b2c3d4e5f67890a1b2c3d4e5f67890")

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "32 bytes")
}

func TestLoad_Production_AcceptsFreshKey(t *testing.T) {
	setBaseEnv(t, "production")
	os.Setenv("FIELD_ENCRYPTION_KEY", validRealKey)

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, validRealKey, cfg.FieldEncryptionKey)
}

func TestLoad_Development_AcceptsPlaceholderWithWarning(t *testing.T) {
	setBaseEnv(t, "development")
	os.Setenv("FIELD_ENCRYPTION_KEY", placeholderKey)

	// Dev tolerates the placeholder (so `docker-compose up` still works out of
	// the box) but the validator emits a log warning. We only assert no error.
	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, placeholderKey, cfg.FieldEncryptionKey)
}

func TestLoad_Test_AllowsEmptyKey(t *testing.T) {
	setBaseEnv(t, "test")
	// FIELD_ENCRYPTION_KEY intentionally unset

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Empty(t, cfg.FieldEncryptionKey)
}

// ─── TLS-is-mandatory-everywhere ─────────────────────────────────────────────
// Prompt requirement: "all transport is secured with TLS even on a local
// network." There is no HTTP fallback in any environment, so Load() must
// reject a Config that lacks either TLS_CERT_FILE or TLS_KEY_FILE — in
// development, test, and production alike.

func TestLoad_TLS_RequiredInProduction(t *testing.T) {
	setBaseEnv(t, "production")
	os.Setenv("FIELD_ENCRYPTION_KEY", validRealKey)
	os.Unsetenv("TLS_CERT_FILE")
	os.Unsetenv("TLS_KEY_FILE")

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TLS_CERT_FILE",
		"error must name the missing TLS env var so the operator can fix it")
}

func TestLoad_TLS_RequiredInDevelopment(t *testing.T) {
	setBaseEnv(t, "development")
	os.Setenv("FIELD_ENCRYPTION_KEY", validRealKey)
	os.Unsetenv("TLS_CERT_FILE")
	os.Unsetenv("TLS_KEY_FILE")

	_, err := config.Load()
	require.Error(t, err,
		"development must require TLS — prompt requires TLS on every network path")
	assert.Contains(t, err.Error(), "TLS_CERT_FILE")
}

func TestLoad_TLS_RequiredInTest(t *testing.T) {
	setBaseEnv(t, "test")
	os.Unsetenv("TLS_CERT_FILE")
	os.Unsetenv("TLS_KEY_FILE")

	_, err := config.Load()
	require.Error(t, err,
		"test env must also require TLS — no HTTP fallback anywhere")
	assert.Contains(t, err.Error(), "TLS_CERT_FILE")
}

func TestLoad_TLS_RejectsCertWithoutKey(t *testing.T) {
	setBaseEnv(t, "development")
	os.Setenv("FIELD_ENCRYPTION_KEY", validRealKey)
	os.Setenv("TLS_CERT_FILE", "/some/cert.pem")
	os.Unsetenv("TLS_KEY_FILE")

	_, err := config.Load()
	require.Error(t, err, "cert without key must be rejected — half-configured TLS is not valid")
}

func TestLoad_TLS_RejectsKeyWithoutCert(t *testing.T) {
	setBaseEnv(t, "development")
	os.Setenv("FIELD_ENCRYPTION_KEY", validRealKey)
	os.Unsetenv("TLS_CERT_FILE")
	os.Setenv("TLS_KEY_FILE", "/some/key.pem")

	_, err := config.Load()
	require.Error(t, err, "key without cert must be rejected")
}

func TestLoad_TLS_AcceptsBothSet(t *testing.T) {
	setBaseEnv(t, "development")
	os.Setenv("FIELD_ENCRYPTION_KEY", validRealKey)
	os.Setenv("TLS_CERT_FILE", "/etc/ssl/dev.crt")
	os.Setenv("TLS_KEY_FILE", "/etc/ssl/dev.key")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, "/etc/ssl/dev.crt", cfg.TLSCertFile)
	assert.Equal(t, "/etc/ssl/dev.key", cfg.TLSKeyFile)
}
