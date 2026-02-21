package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"kari/api/internal/core/domain"
)

type EnvVarService struct {
	repo          domain.ApplicationRepository
	cryptoService domain.CryptoService
	logger        *slog.Logger
}

func NewEnvVarService(
	repo domain.ApplicationRepository,
	crypto domain.CryptoService,
	logger *slog.Logger,
) *EnvVarService {
	return &EnvVarService{
		repo:          repo,
		cryptoService: crypto,
		logger:        logger,
	}
}

// UpdateSecrets encrypts and persists application environment variables.
func (s *EnvVarService) UpdateSecrets(ctx context.Context, appID uuid.UUID, vars map[string]string) error {
	// 1. Serialize the map to JSON before encryption
	plaintext, err := json.Marshal(vars)
	if err != nil {
		return fmt.Errorf("failed to serialize env vars: %w", err)
	}

	// 🛡️ 2. AEAD Encryption: Bind to AppID as Associated Data
	// This ensures that even if a database row is leaked, the secret 
	// cannot be decrypted and used for a DIFFERENT application.
	ciphertext, err := s.cryptoService.Encrypt(ctx, plaintext, appID.NodeID())
	if err != nil {
		s.logger.Error("Encryption failure", slog.String("app_id", appID.String()))
		return fmt.Errorf("cryptographic failure")
	}

	// 3. Persist the ciphertext (The repo handles the JSONB storage)
	// We store it in a map format that the repo expects for the env_vars column.
	encryptedMap := map[string]string{
		"data": ciphertext,
	}

	return s.repo.UpdateEnvVars(ctx, appID, encryptedMap)
}

// GetDecryptedVars retrieves and decrypts the secrets for the Rust Muscle.
func (s *EnvVarService) GetDecryptedVars(ctx context.Context, appID uuid.UUID, userID uuid.UUID) (map[string]string, error) {
	app, err := s.repo.GetByID(ctx, appID, userID)
	if err != nil {
		return nil, err
	}

	ciphertext, ok := app.EnvVars["data"]
	if !ok {
		return make(map[string]string), nil
	}

	// 🛡️ 4. Decrypt with the same AppID binding
	plaintext, err := s.cryptoService.Decrypt(ctx, ciphertext, appID.NodeID())
	if err != nil {
		return nil, fmt.Errorf("integrity violation: failed to decrypt secrets")
	}

	var decryptedVars map[string]string
	if err := json.Unmarshal(plaintext, &decryptedVars); err != nil {
		return nil, fmt.Errorf("failed to parse decrypted secrets: %w", err)
	}

	return decryptedVars, nil
}
