\package crypto

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

// 🛡️ SLA: Domain Interface (Should live in core/domain/interfaces.go)
type CryptoService interface {
	Encrypt(ctx context.Context, plaintext []byte, associatedData []byte) (string, error)
	Decrypt(ctx context.Context, ciphertextBase64 string, associatedData []byte) ([]byte, error)
}

type AESCryptoService struct {
	// 🛡️ Optimized: Pre-calculate the AEAD interface to reduce allocations
	aead cipher.AEAD
}

func NewAESCryptoService(hexKey string) (*AESCryptoService, error) {
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: invalid key encoding: %w", err)
	}

	if len(key) != 32 {
		return nil, errors.New("crypto: key must be 32 bytes for AES-256")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: block cipher failure: %w", err)
	}

	// 🛡️ Privacy Tip: Manually zeroize the temporary key slice after use
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

func (s *AESCryptoService) Encrypt(ctx context.Context, plaintext []byte, associatedData []byte) (string, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("crypto: nonce generation failure: %w", err)
	}

	// 🛡️ Memory Safety: Pre-allocate the exact size needed
	// Capacity = nonce + plaintext + tag (usually 16 bytes)
	ciphertext := s.aead.Seal(nonce, nonce, plaintext, associatedData)
	
	return base64.URLEncoding.EncodeToString(ciphertext), nil
}

func (s *AESCryptoService) Decrypt(ctx context.Context, ciphertextBase64 string, associatedData []byte) ([]byte, error) {
	data, err := base64.URLEncoding.DecodeString(ciphertextBase64)
	if err != nil {
		return nil, fmt.Errorf("crypto: base64 decode failure: %w", err)
	}

	ns := s.aead.NonceSize()
	if len(data) < ns {
		return nil, errors.New("crypto: ciphertext too short")
	}

	nonce, actualCiphertext := data[:ns], data[ns:]

	// 🛡️ AEAD Verification
	// If the AppID or UserID used as 'associatedData' changed, this WILL fail.
	plaintext, err := s.aead.Open(nil, nonce, actualCiphertext, associatedData)
	if err != nil {
		return nil, fmt.Errorf("crypto: integrity violation - potential tampering detected")
	}

	return plaintext, nil
}
