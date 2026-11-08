package services

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"image/png"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/irgordon/kari/api/internal/core/domain"
	"github.com/irgordon/kari/api/internal/db/postgres"
	"github.com/irgordon/kari/api/internal/infrastructure/crypto"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

type MFAService struct {
	Repo   *postgres.UserRepo
	Auth   *AuthService
	Crypto *crypto.AESCryptoService
}

type MFAEnrollment struct {
	Secret string `json:"secret"`
	URI    string `json:"uri"`
	QR     string `json:"qr"`
}

func (s *MFAService) Begin(ctx context.Context, user *domain.User, password string) (*MFAEnrollment, error) {
	if user.MFAEnabled || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return nil, domain.ErrInvalidCredentials
	}
	key, err := totp.Generate(totp.GenerateOpts{Issuer: "Kari", AccountName: user.Email, Period: 30, Digits: otp.DigitsSix, SecretSize: 20})
	if err != nil {
		return nil, err
	}
	cipher, err := s.Crypto.Encrypt(ctx, []byte(key.Secret()), []byte("mfa:"+user.ID.String()))
	if err != nil {
		return nil, err
	}
	if err = s.Repo.BeginMFA(ctx, user.ID, user.CredentialVersion, cipher); err != nil {
		return nil, err
	}
	image, err := key.Image(200, 200)
	if err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	if err = png.Encode(&buffer, image); err != nil {
		return nil, err
	}
	return &MFAEnrollment{Secret: key.Secret(), URI: key.URL(), QR: "data:image/png;base64," + base64.StdEncoding.EncodeToString(buffer.Bytes())}, nil
}

func (s *MFAService) counter(ctx context.Context, id uuid.UUID, cipher, code string) (int64, error) {
	if cipher == "" {
		return -1, domain.ErrInvalidCredentials
	}
	secret, err := s.Crypto.Decrypt(ctx, cipher, []byte("mfa:"+id.String()))
	if err != nil {
		return -1, err
	}
	defer func() {
		for i := range secret {
			secret[i] = 0
		}
	}()
	now := time.Now().Unix() / 30
	for _, offset := range []int64{0, -1, 1} {
		valid, err := totp.ValidateCustom(code, string(secret), time.Unix((now+offset)*30, 0), totp.ValidateOpts{Period: 30, Skew: 0, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
		if err == nil && valid {
			return now + offset, nil
		}
	}
	return -1, nil
}

func (s *MFAService) Change(ctx context.Context, user *domain.User, password, code string, enable bool) (*domain.TokenPair, []string, error) {
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return nil, nil, domain.ErrInvalidCredentials
	}
	state, err := s.Repo.MFAState(ctx, user.ID)
	if err != nil {
		return nil, nil, err
	}
	if state.Version != user.CredentialVersion {
		return nil, nil, domain.ErrInvalidCredentials
	}
	cipher := state.Secret
	if enable {
		cipher = state.Pending
	}
	counter, err := s.counter(ctx, user.ID, cipher, code)
	if err != nil {
		return nil, nil, err
	}
	if counter < 0 {
		return nil, nil, domain.ErrInvalidCredentials
	}
	var codes, hashes []string
	if enable {
		for i := 0; i < 10; i++ {
			entropy := make([]byte, 16)
			if _, err = rand.Read(entropy); err != nil {
				return nil, nil, err
			}
			raw := strings.ToUpper(hex.EncodeToString(entropy))
			codes = append(codes, raw[:8]+"-"+raw[8:16]+"-"+raw[16:24]+"-"+raw[24:])
			hashes = append(hashes, refreshHash(raw))
		}
	}
	updated := *user
	updated.CredentialVersion++
	updated.MFAEnabled = enable
	updated.MFAVerified = enable
	id := uuid.New()
	tokens, err := s.Auth.sessionTokens(&updated, id)
	if err != nil {
		return nil, nil, err
	}
	session := &domain.Session{ID: id, UserID: user.ID, CredentialVersion: updated.CredentialVersion, MFAVerified: enable, RefreshHash: refreshHash(tokens.RefreshToken), ExpiresAt: time.Now().Add(7 * 24 * time.Hour)}
	pending := ""
	if enable {
		pending = state.Pending
	}
	err = s.Repo.ChangeMFA(ctx, user.ID, state.Version, pending, counter, hashes, session)
	return tokens, codes, err
}

func (s *MFAService) Complete(ctx context.Context, challenge, code string) (*domain.TokenPair, *domain.User, error) {
	if len(challenge) != 43 {
		return nil, nil, domain.ErrInvalidCredentials
	}
	session, err := s.Repo.PendingMFA(ctx, refreshHash(challenge))
	if err != nil {
		return nil, nil, err
	}
	user, err := s.Repo.GetByID(ctx, session.UserID)
	if err != nil {
		return nil, nil, err
	}
	state, err := s.Repo.MFAState(ctx, user.ID)
	if err != nil {
		return nil, nil, err
	}
	counter, err := s.counter(ctx, user.ID, state.Secret, code)
	if err != nil {
		return nil, nil, err
	}
	recoveryHash := ""
	normalized := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), "-", ""))
	if len(normalized) == 32 {
		recoveryHash = refreshHash(normalized)
	}
	user.MFAVerified = true
	tokens, err := s.Auth.sessionTokens(user, session.ID)
	if err != nil {
		return nil, nil, err
	}
	session.RefreshHash = refreshHash(tokens.RefreshToken)
	session.ExpiresAt = time.Now().Add(7 * 24 * time.Hour)
	err = s.Repo.CompleteMFA(ctx, refreshHash(challenge), session, counter, recoveryHash)
	return tokens, user, err
}
