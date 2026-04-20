package crypto_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/eagle-point/service-portal/internal/crypto"
)

func TestSign_Deterministic(t *testing.T) {
	secret := []byte("my-hmac-secret")
	msg := "GET\n/api/v1/data\nabc123"

	sig1 := crypto.Sign(msg, secret)
	sig2 := crypto.Sign(msg, secret)
	assert.Equal(t, sig1, sig2)
}

func TestVerify_Valid(t *testing.T) {
	secret := []byte("my-hmac-secret")
	msg := "POST\n/api/v1/ingest\nbodyhash"

	sig := crypto.Sign(msg, secret)
	header := crypto.FormatHeader(sig)

	assert.True(t, crypto.Verify(msg, header, secret))
}

func TestVerify_TamperedMessage(t *testing.T) {
	secret := []byte("my-hmac-secret")
	msg := "POST\n/api/v1/ingest\nbodyhash"
	tampered := "POST\n/api/v1/ingest\ndifferenthash"

	sig := crypto.Sign(msg, secret)
	header := crypto.FormatHeader(sig)

	assert.False(t, crypto.Verify(tampered, header, secret))
}

func TestVerify_WrongSecret(t *testing.T) {
	msg := "POST\n/api/data\nhash"
	sig := crypto.Sign(msg, []byte("secret1"))
	header := crypto.FormatHeader(sig)

	assert.False(t, crypto.Verify(msg, header, []byte("secret2")))
}

func TestVerify_MalformedHeader(t *testing.T) {
	secret := []byte("secret")
	msg := "GET\n/path\nhash"

	assert.False(t, crypto.Verify(msg, "bad-header", secret))
	assert.False(t, crypto.Verify(msg, "wrong-scheme abc123", secret))
	assert.False(t, crypto.Verify(msg, "", secret))
}
