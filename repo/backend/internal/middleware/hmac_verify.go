package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/eagle-point/service-portal/internal/apierr"
	appCrypto "github.com/eagle-point/service-portal/internal/crypto"
)

const (
	hmacKeyIDHeader = "X-Key-ID"
	hmacSigHeader   = "X-Signature"
)

// HMACVerifier validates HMAC-signed requests from internal clients.
type HMACVerifier struct {
	db     *sql.DB
	encKey string // AES-256 key (hex) for decrypting stored HMAC secrets
}

// NewHMACVerifier creates an HMACVerifier.
func NewHMACVerifier(db *sql.DB, encKey string) *HMACVerifier {
	return &HMACVerifier{db: db, encKey: encKey}
}

// ValidateHMAC returns a middleware that verifies HMAC-SHA256 request signatures.
// Signing message: METHOD + "\n" + PATH + "\n" + hex(sha256(body))
func (hv *HMACVerifier) ValidateHMAC() gin.HandlerFunc {
	return func(c *gin.Context) {
		keyID := c.GetHeader(hmacKeyIDHeader)
		sigHeader := c.GetHeader(hmacSigHeader)

		if keyID == "" || sigHeader == "" {
			apierr.BadRequest(c, "hmac_missing", "X-Key-ID and X-Signature headers are required")
			return
		}

		secret, err := hv.loadSecret(c.Request.Context(), keyID)
		if err != nil {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			apierr.InternalError(c)
			return
		}
		// Restore body so downstream handlers can read it
		c.Request.Body = io.NopCloser(bytes.NewReader(body))

		msg := buildHMACMessage(c.Request.Method, c.Request.URL.Path, body)

		if !appCrypto.Verify(msg, sigHeader, secret) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{"code": "hmac_invalid", "message": "invalid request signature"},
			})
			return
		}

		c.Next()
	}
}

func (hv *HMACVerifier) loadSecret(ctx context.Context, keyID string) ([]byte, error) {
	var encrypted []byte
	err := hv.db.QueryRowContext(ctx,
		`SELECT secret_encrypted FROM hmac_keys WHERE key_id = ? AND is_active = 1`, keyID,
	).Scan(&encrypted)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("hmac: key not found: %s", keyID)
	}
	if err != nil {
		return nil, fmt.Errorf("hmac: db error: %w", err)
	}

	secret, err := appCrypto.Decrypt(encrypted, hv.encKey)
	if err != nil {
		return nil, fmt.Errorf("hmac: decrypt secret: %w", err)
	}
	return secret, nil
}

func buildHMACMessage(method, path string, body []byte) string {
	h := sha256.Sum256(body)
	return fmt.Sprintf("%s\n%s\n%s", method, path, hex.EncodeToString(h[:]))
}
