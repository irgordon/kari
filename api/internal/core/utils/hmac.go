package utils

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strings"
)

const (
	// Maximum webhook payload size (10MB).
	// This prevents memory exhaustion attacks.
	MaxWebhookBodySize = 10 * 1024 * 1024
)

// VerifyGitHubSignature calculates the HMAC of the raw body and compares it
// against the X-Hub-Signature-256 header in constant time.
func VerifyGitHubSignature(rawBody []byte, signatureHeader string, secret []byte) error {
	// 🛡️ 1. Sanity & Entropy Checks
	if len(secret) < 16 {
		return errors.New("webhook secret entropy too low")
	}

	if signatureHeader == "" {
		return errors.New("missing signature header")
	}

	// 🛡️ 2. Formatting Guard
	// GitHub sends: "sha256=HEX_DIGEST"
	const prefix = "sha256="
	if !strings.HasPrefix(signatureHeader, prefix) {
		return errors.New("unsupported signature algorithm")
	}

	actualSig := strings.TrimPrefix(signatureHeader, prefix)
	providedMAC, err := hex.DecodeString(actualSig)
	if err != nil {
		return errors.New("invalid signature encoding")
	}

	// 🛡️ 3. HMAC Computation
	// Use hmac.New(sha256.New, ...) which is standard for GitHub 2026.
	mac := hmac.New(sha256.New, secret)
	mac.Write(rawBody)
	expectedMAC := mac.Sum(nil)

	// 🛡️ 4. Secure Comparison
	// subtle.ConstantTimeCompare returns 1 only if slices are equal.
	// This is mathematically hardened against side-channel timing analysis.
	if subtle.ConstantTimeCompare(expectedMAC, providedMAC) != 1 {
		return errors.New("webhook signature mismatch")
	}

	return nil
}
