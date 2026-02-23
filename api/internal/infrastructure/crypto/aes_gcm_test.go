package crypto_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"

	"kari/api/internal/infrastructure/crypto"
)

// generateTestKey creates a random 256-bit AES key in hex
func generateTestKey(t *testing.T) string {
	t.Helper()
	key := make([]byte, 32) // 256-bit
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}
	return hex.EncodeToString(key)
}

// ==============================================================================
// 1. Fundamental Correctness
// ==============================================================================

func TestAESGCM_EncryptDecrypt_RoundTrip(t *testing.T) {
	svc, err := crypto.NewAESCryptoService(generateTestKey(t))
	if err != nil {
		t.Fatalf("Failed to create crypto service: %v", err)
	}

	ctx := context.Background()
	plaintext := []byte("ssh-rsa AAAAB3NzaC1yc2E... deploy@kari.dev")
	aad := []byte("app-uuid-1234-5678")

	ciphertext, err := svc.Encrypt(ctx, plaintext, aad)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	decrypted, err := svc.Decrypt(ctx, ciphertext, aad)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("Round-trip failed: got %q, want %q", decrypted, plaintext)
	}
}

// ==============================================================================
// 2. AAD Binding Verification (Mathematical Proof)
// ==============================================================================

func TestAESGCM_AAD_Tamper_Detection(t *testing.T) {
	svc, err := crypto.NewAESCryptoService(generateTestKey(t))
	if err != nil {
		t.Fatalf("Failed to create crypto service: %v", err)
	}

	ctx := context.Background()
	plaintext := []byte("SUPER_SECRET_DATABASE_PASSWORD")

	// Encrypt with AAD bound to AppID "good-app"
	ciphertext, err := svc.Encrypt(ctx, plaintext, []byte("good-app"))
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// 🛡️ CRITICAL TEST: Attempt to decrypt with DIFFERENT AAD ("evil-app")
	// GCM should reject this because the authentication tag won't verify.
	_, err = svc.Decrypt(ctx, ciphertext, []byte("evil-app"))
	if err == nil {
		t.Fatal("SECURITY VIOLATION: Decrypt succeeded with tampered AAD — AAD binding is broken")
	}

	// Verify the CORRECT AAD still works
	decrypted, err := svc.Decrypt(ctx, ciphertext, []byte("good-app"))
	if err != nil {
		t.Fatalf("Decrypt with correct AAD failed: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Errorf("AAD round-trip failed: got %q, want %q", decrypted, plaintext)
	}
}

// ==============================================================================
// 3. Nonce Uniqueness (Semantic Security)
// ==============================================================================

func TestAESGCM_Nonce_Uniqueness(t *testing.T) {
	svc, err := crypto.NewAESCryptoService(generateTestKey(t))
	if err != nil {
		t.Fatalf("Failed to create crypto service: %v", err)
	}

	ctx := context.Background()
	plaintext := []byte("identical-plaintext")
	aad := []byte("same-aad")

	// Encrypt the SAME plaintext 100 times
	ciphertexts := make(map[string]bool)
	for i := 0; i < 100; i++ {
		ct, err := svc.Encrypt(ctx, plaintext, aad)
		if err != nil {
			t.Fatalf("Encrypt #%d failed: %v", i, err)
		}
		if ciphertexts[ct] {
			t.Fatalf("SECURITY VIOLATION: Nonce reuse detected at iteration %d — identical ciphertext produced", i)
		}
		ciphertexts[ct] = true
	}
}

// ==============================================================================
// 4. Key Validation
// ==============================================================================

func TestAESGCM_Rejects_Short_Key(t *testing.T) {
	// 128-bit key (should require 256-bit)
	shortKey := strings.Repeat("ab", 16) // 32 hex chars = 128 bits
	_, err := crypto.NewAESCryptoService(shortKey)
	if err == nil {
		t.Fatal("SECURITY VIOLATION: Accepted 128-bit key — must require 256-bit")
	}
}

func TestAESGCM_Rejects_Invalid_Hex(t *testing.T) {
	_, err := crypto.NewAESCryptoService("not-a-valid-hex-string-at-all!!!")
	if err == nil {
		t.Fatal("SECURITY VIOLATION: Accepted non-hex key")
	}
}

func TestAESGCM_Rejects_Empty_Key(t *testing.T) {
	_, err := crypto.NewAESCryptoService("")
	if err == nil {
		t.Fatal("SECURITY VIOLATION: Accepted empty key")
	}
}

// ==============================================================================
// 5. Ciphertext Tampering Detection
// ==============================================================================

func TestAESGCM_Ciphertext_Tamper_Detection(t *testing.T) {
	svc, err := crypto.NewAESCryptoService(generateTestKey(t))
	if err != nil {
		t.Fatalf("Failed to create crypto service: %v", err)
	}

	ctx := context.Background()
	plaintext := []byte("sensitive-data")
	aad := []byte("bound-context")

	ciphertext, err := svc.Encrypt(ctx, plaintext, aad)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// 🛡️ Tamper with the ciphertext (flip a character)
	tampered := []byte(ciphertext)
	if len(tampered) > 10 {
		if tampered[10] == 'a' {
			tampered[10] = 'b'
		} else {
			tampered[10] = 'a'
		}
	}

	_, err = svc.Decrypt(ctx, string(tampered), aad)
	if err == nil {
		t.Fatal("SECURITY VIOLATION: Decrypt succeeded with tampered ciphertext — GCM auth tag not verified")
	}
}

// ==============================================================================
// 6. Empty Plaintext Edge Case
// ==============================================================================

func TestAESGCM_Empty_Plaintext(t *testing.T) {
	svc, err := crypto.NewAESCryptoService(generateTestKey(t))
	if err != nil {
		t.Fatalf("Failed to create crypto service: %v", err)
	}

	ctx := context.Background()

	// GCM should handle empty plaintext (produces only the auth tag)
	ciphertext, err := svc.Encrypt(ctx, []byte{}, []byte("aad"))
	if err != nil {
		t.Fatalf("Encrypt empty plaintext failed: %v", err)
	}

	decrypted, err := svc.Decrypt(ctx, ciphertext, []byte("aad"))
	if err != nil {
		t.Fatalf("Decrypt empty plaintext failed: %v", err)
	}

	if len(decrypted) != 0 {
		t.Errorf("Expected empty plaintext, got %d bytes", len(decrypted))
	}
}
