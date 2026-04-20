package config_test

// Static coverage for the "TLS in every environment" policy.
//
// The per-environment regression matrix lives in config_test.go alongside
// the other Load() cases. This file adds higher-level assertions about the
// shape of the error (so contributors can rely on the message containing
// actionable guidance) and pins the default-env no-env-vars case.

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eagle-point/service-portal/internal/config"
)

func TestTLSPolicy_ErrorMessage_NamesCertAndKey(t *testing.T) {
	setBaseEnv(t, "development")
	os.Setenv("FIELD_ENCRYPTION_KEY", validRealKey)
	os.Unsetenv("TLS_CERT_FILE")
	os.Unsetenv("TLS_KEY_FILE")

	_, err := config.Load()
	require.Error(t, err)
	msg := err.Error()
	assert.True(t,
		strings.Contains(msg, "TLS_CERT_FILE") && strings.Contains(msg, "TLS_KEY_FILE"),
		"error must name both TLS env vars; got: %s", msg)
}

func TestTLSPolicy_ErrorMessage_PointsAtCertGeneratorScript(t *testing.T) {
	setBaseEnv(t, "test")
	os.Unsetenv("TLS_CERT_FILE")
	os.Unsetenv("TLS_KEY_FILE")

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gen-tls-cert.sh",
		"message must point at the dev cert generator so developers have a one-shot fix")
}

func TestTLSPolicy_NoHTTPFallback_AnyEnvRefused(t *testing.T) {
	// Iterate through the three supported APP_ENV values. For each, Load()
	// must refuse if TLS_CERT_FILE/TLS_KEY_FILE are absent. Catches any
	// regression that reintroduces a per-environment exception.
	for _, env := range []string{"production", "development", "test"} {
		t.Run(env, func(t *testing.T) {
			setBaseEnv(t, env)
			if env == "production" {
				os.Setenv("FIELD_ENCRYPTION_KEY", validRealKey)
			}
			os.Unsetenv("TLS_CERT_FILE")
			os.Unsetenv("TLS_KEY_FILE")

			_, err := config.Load()
			require.Error(t, err,
				"%s env must refuse to Load() without TLS cert+key", env)
		})
	}
}
