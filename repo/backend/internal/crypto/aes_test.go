package crypto_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eagle-point/service-portal/internal/crypto"
)

const testKey = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	cases := []string{
		"hello world",
		"(415) 555-1234",
		"123 Main St",
		"",
		"Unicode: 日本語テスト",
	}

	for _, tc := range cases {
		t.Run(tc, func(t *testing.T) {
			enc, err := crypto.EncryptString(tc, testKey)
			require.NoError(t, err)
			require.NotNil(t, enc)

			dec, err := crypto.DecryptString(enc, testKey)
			require.NoError(t, err)
			assert.Equal(t, tc, dec)
		})
	}
}

func TestEncrypt_UniqueEachCall(t *testing.T) {
	plain := "same plaintext"
	enc1, err := crypto.EncryptString(plain, testKey)
	require.NoError(t, err)
	enc2, err := crypto.EncryptString(plain, testKey)
	require.NoError(t, err)

	// Different nonces → different ciphertext
	assert.False(t, bytes.Equal(enc1, enc2))
}

func TestDecrypt_TamperedData(t *testing.T) {
	enc, err := crypto.EncryptString("secret", testKey)
	require.NoError(t, err)

	// Flip a byte in ciphertext
	enc[len(enc)-1] ^= 0xFF
	_, err = crypto.Decrypt(enc, testKey)
	assert.Error(t, err)
}

func TestDecrypt_WrongKey(t *testing.T) {
	enc, err := crypto.EncryptString("secret", testKey)
	require.NoError(t, err)

	wrongKey := "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	_, err = crypto.Decrypt(enc, wrongKey)
	assert.Error(t, err)
}

func TestDecodeKey_InvalidHex(t *testing.T) {
	_, err := crypto.EncryptString("data", "not-hex")
	assert.Error(t, err)
}

func TestDecodeKey_WrongLength(t *testing.T) {
	_, err := crypto.EncryptString("data", "0102030405") // too short
	assert.Error(t, err)
}
