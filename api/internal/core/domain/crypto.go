package domain

import "context"

// CryptoService defines the hardened contract for secret management.
// It enforces AEAD (Authenticated Encryption with Associated Data).
type CryptoService interface {
	// Encrypt transforms plaintext into an authenticated ciphertext.
	// 'associatedData' (AAD) links the secret to a specific context (e.g., AppID).
	Encrypt(ctx context.Context, plaintext []byte, associatedData []byte) (string, error)

	// Decrypt verifies authenticity and returns the original plaintext.
	// If the AAD does not match what was used during encryption, it returns an error.
	Decrypt(ctx context.Context, ciphertextBase64 string, associatedData []byte) ([]byte, error)
}
