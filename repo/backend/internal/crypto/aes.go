package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// Encrypt encrypts plaintext with AES-256-GCM.
// keyHex must be a 64-character hex-encoded 32-byte key.
// Returns raw bytes: nonce (12 bytes) || ciphertext+tag.
// Store result in VARBINARY columns.
func Encrypt(plaintext []byte, keyHex string) ([]byte, error) {
	key, err := decodeKey(keyHex)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("crypto: generate nonce: %w", err)
	}

	// Seal appends ciphertext+tag to nonce
	out := gcm.Seal(nonce, nonce, plaintext, nil)
	return out, nil
}

// Decrypt decrypts AES-256-GCM ciphertext produced by Encrypt.
func Decrypt(data []byte, keyHex string) ([]byte, error) {
	key, err := decodeKey(keyHex)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, errors.New("crypto: ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt failed: %w", err)
	}

	return plaintext, nil
}

// EncryptString is a convenience wrapper for string values.
func EncryptString(plaintext string, keyHex string) ([]byte, error) {
	return Encrypt([]byte(plaintext), keyHex)
}

// DecryptString is a convenience wrapper that returns a string.
func DecryptString(data []byte, keyHex string) (string, error) {
	b, err := Decrypt(data, keyHex)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func decodeKey(keyHex string) ([]byte, error) {
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, fmt.Errorf("crypto: invalid key hex: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("crypto: key must be 32 bytes, got %d", len(key))
	}
	return key, nil
}
