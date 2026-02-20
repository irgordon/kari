package utils

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strings"
)

// VerifyGitHubSignature calculates the HMAC of the raw body and compares it 
// against the X-Hub-Signature-256 header in constant time to prevent timing attacks.
func VerifyGitHubSignature(rawBody []byte, signatureHeader string, secret string) error {
	if signatureHeader == "" {
		return errors.New("missing signature header")
	}

	// GitHub sends the header in the format: "sha256=1234567890abcdef..."
	parts := strings.SplitN(signatureHeader, "=", 2)
	if len(parts) != 2 || parts[0] != "sha256" {
		return errors.New("invalid signature format")
	}

	providedMAC, err := hex.DecodeString(parts[1])
	if err != nil {
		return errors.New("invalid signature encoding")
	}

	// Calculate the expected MAC using our stored secret
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(rawBody)
	expectedMAC := mac.Sum(nil)

	// Secure by Design: Constant-time comparison defeats timing attacks
	if subtle.ConstantTimeCompare(expectedMAC, providedMAC) != 1 {
		return errors.New("signature mismatch")
	}

	return nil
}
