package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

const HMACScheme = "hmac-sha256"

// Sign returns an HMAC-SHA256 hex signature over the message.
// message is typically: method + "\n" + path + "\n" + body_sha256_hex
func Sign(message string, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify returns true if the signature header matches the computed HMAC.
// sigHeader format: "hmac-sha256 <hex-signature>"
func Verify(message string, sigHeader string, secret []byte) bool {
	parts := strings.SplitN(sigHeader, " ", 2)
	if len(parts) != 2 || parts[0] != HMACScheme {
		return false
	}
	expected := Sign(message, secret)
	return hmac.Equal([]byte(parts[1]), []byte(expected))
}

// FormatHeader builds the X-Signature header value.
func FormatHeader(sig string) string {
	return fmt.Sprintf("%s %s", HMACScheme, sig)
}
