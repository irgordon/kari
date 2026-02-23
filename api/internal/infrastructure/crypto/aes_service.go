package crypto

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// 🛡️ SLA: Domain Interface
type CryptoService interface {
	Encrypt(ctx context.Context, plaintext []byte, associatedData []byte) (string, error)
	Decrypt(ctx context.Context, ciphertextBase64 string, associatedData []byte) ([]byte, error)
}

type AESCryptoService struct {
	// 🛡️ Optimized: Pre-calculate the AEAD interface to reduce allocations
	aead cipher.AEAD
}

// NewAESCryptoService initializes the high-performance AES-GCM cipher block.
func NewAESCryptoService(hexKey string) (*AESCryptoService, error) {
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: invalid key encoding: %w", err)
	}

	if len(key) != 32 {
		return nil, errors.New("crypto: key must be exactly 32 bytes for AES-256")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: block cipher failure: %w", err)
	}

	// Best-effort Go memory hygiene for the initial decode slice
	defer func() {
		for i := range key {
			key[i] = 0
		}
	}()

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: GCM failure: %w", err)
	}

	return &AESCryptoService{aead: aesGCM}, nil
}

// Encrypt secures the payload with zero extra heap allocations during the Seal phase.
func (s *AESCryptoService) Encrypt(ctx context.Context, plaintext []byte, associatedData []byte) (string, error) {
	// Acknowledge the context for interface compliance (e.g., tracing could be added here)
	_ = ctx 

	nonceSize := s.aead.NonceSize()
	
	// 1. 🛡️ TRUE Performance: Exact Capacity Pre-allocation
	// We create a slice where Length = nonceSize, but Capacity = nonceSize + len(plaintext) + tag size.
	// This mathematically guarantees `Seal` will append without triggering a slice grow/reallocation.
	buf := make([]byte, nonceSize, nonceSize+len(plaintext)+s.aead.Overhead())

	// 2. 🛡️ Entropy: Fill just the nonce portion
	if _, err := io.ReadFull(rand.Reader, buf[:nonceSize]); err != nil {
		return "", fmt.Errorf("crypto: nonce generation failure: %w", err)
	}

	// 3. 🛡️ Authenticated Sealing
	// Seal appends to the slice up to its capacity limit.
	ciphertext := s.aead.Seal(buf[:nonceSize], buf[:nonceSize], plaintext, associatedData)
	
	return base64.URLEncoding.EncodeToString(ciphertext), nil
}

// Decrypt verifies the AAD signature and returns the plaintext.
func (s *AESCryptoService) Decrypt(ctx context.Context, ciphertextBase64 string, associatedData []byte) ([]byte, error) {
	_ = ctx

	data, err := base64.URLEncoding.DecodeString(ciphertextBase64)
	if err != nil {
		return nil, fmt.Errorf("crypto: base64 decode failure: %w", err)
	}

	ns := s.aead.NonceSize()
	if len(data) < ns {
		return nil, errors.New("crypto: ciphertext too short")
	}

	nonce, actualCiphertext := data[:ns], data[ns:]

	// 🛡️ AEAD Verification (Zero-Trust Context Binding)
	// If the database was tampered with, or if the associatedData (e.g., AppID) doesn't match,
	// this instantly fails and refuses to return the manipulated payload.
	plaintext, err := s.aead.Open(nil, nonce, actualCiphertext, associatedData)
	if err != nil {
		return nil, errors.New("crypto: integrity violation - potential tampering detected")
	}

	return plaintext, nil
}
