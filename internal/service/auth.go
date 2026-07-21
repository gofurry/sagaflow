package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gofurry/easyhash"
	"github.com/gofurry/sagaflow/internal/config"
	"github.com/gofurry/sagaflow/internal/store/db"
	"github.com/google/uuid"
)

type Principal struct {
	AccountID   uuid.UUID `json:"account_id"`
	SessionID   uuid.UUID `json:"session_id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
}

type AuthService struct {
	store *db.Store
	cfg   config.AuthConfig
	ttl   time.Duration
}

func NewAuthService(store *db.Store, cfg config.AuthConfig) (*AuthService, error) {
	ttl := 30 * 24 * time.Hour
	if cfg.SessionTTL != "" {
		parsed, err := time.ParseDuration(cfg.SessionTTL)
		if err != nil {
			return nil, fmt.Errorf("parse auth session ttl: %w", err)
		}
		ttl = parsed
	}
	return &AuthService{store: store, cfg: cfg, ttl: ttl}, nil
}

func (s *AuthService) Enabled() bool { return true }
func (s *AuthService) CookieName() string {
	if s == nil || s.cfg.CookieName == "" {
		return "sagaflow_session"
	}
	return s.cfg.CookieName
}
func (s *AuthService) CookieSecure() bool { return s != nil && s.cfg.CookieSecure }
func (s *AuthService) TokenTTL() time.Duration {
	if s == nil || s.ttl <= 0 {
		return 30 * 24 * time.Hour
	}
	return s.ttl
}
func (s *AuthService) Initialized(ctx context.Context) (bool, error) {
	if s == nil || s.store == nil {
		return false, ErrAuthNotInitialized
	}
	return s.store.AuthInitialized(ctx)
}

func (s *AuthService) Initialize(ctx context.Context, username, displayName, password string) (db.Account, error) {
	username = strings.TrimSpace(username)
	displayName = strings.TrimSpace(displayName)
	if username == "" || password == "" {
		return db.Account{}, fmt.Errorf("%w: username and password are required", ErrInvalidInput)
	}
	if displayName == "" {
		displayName = username
	}
	hash, err := hashPassword(password)
	if err != nil {
		return db.Account{}, err
	}
	return s.store.CreateAccount(ctx, username, displayName, hash)
}

func (s *AuthService) SetPassword(ctx context.Context, password string) (db.Account, error) {
	hash, err := hashPassword(password)
	if err != nil {
		return db.Account{}, err
	}
	if initialized, err := s.Initialized(ctx); err != nil {
		return db.Account{}, err
	} else if !initialized {
		return s.store.CreateAccount(ctx, "admin", "Creator", hash)
	}
	if err := s.store.RevokeAllSessions(ctx); err != nil {
		return db.Account{}, err
	}
	return s.store.UpdateAccountPassword(ctx, hash)
}

func (s *AuthService) Login(ctx context.Context, username, password string) (string, time.Time, Principal, error) {
	if strings.TrimSpace(username) == "" || password == "" {
		return "", time.Time{}, Principal{}, ErrInvalidCredentials
	}
	account, err := s.store.GetAccountByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return "", time.Time{}, Principal{}, ErrInvalidCredentials
		}
		return "", time.Time{}, Principal{}, err
	}
	ok, upgradedHash, upgraded, err := easyhash.VerifyAndUpgrade(password, account.PasswordHash, easyhash.DefaultPolicy())
	if err != nil || !ok {
		return "", time.Time{}, Principal{}, ErrInvalidCredentials
	}
	if upgraded {
		if _, err := s.store.UpdateAccountPassword(ctx, upgradedHash); err != nil {
			return "", time.Time{}, Principal{}, err
		}
	}
	token, tokenHash, err := newSessionToken()
	if err != nil {
		return "", time.Time{}, Principal{}, err
	}
	expiresAt := time.Now().UTC().Add(s.TokenTTL())
	session, err := s.store.CreateSession(ctx, tokenHash, expiresAt)
	if err != nil {
		return "", time.Time{}, Principal{}, err
	}
	if err := s.store.TouchAccountLogin(ctx); err != nil {
		return "", time.Time{}, Principal{}, err
	}
	return token, expiresAt, principalFrom(account, session.ID), nil
}

func (s *AuthService) AuthenticateToken(ctx context.Context, token string) (Principal, error) {
	if strings.TrimSpace(token) == "" {
		return Principal{}, ErrUnauthorized
	}
	session, err := s.store.GetActiveSessionByTokenHash(ctx, sessionTokenHash(token))
	if err != nil {
		return Principal{}, ErrUnauthorized
	}
	account, err := s.store.GetAccount(ctx)
	if err != nil {
		return Principal{}, ErrUnauthorized
	}
	return principalFrom(account, session.ID), nil
}

func (s *AuthService) Logout(ctx context.Context, principal Principal) error {
	if principal.SessionID == uuid.Nil {
		return nil
	}
	return s.store.RevokeSession(ctx, principal.SessionID)
}

func (s *AuthService) ChangePassword(ctx context.Context, principal Principal, current, next string) (db.Account, error) {
	account, err := s.store.GetAccount(ctx)
	if err != nil || account.ID != principal.AccountID {
		return db.Account{}, ErrUnauthorized
	}
	ok, err := easyhash.Verify(current, account.PasswordHash)
	if err != nil || !ok {
		return db.Account{}, ErrInvalidCredentials
	}
	hash, err := hashPassword(next)
	if err != nil {
		return db.Account{}, err
	}
	if err := s.store.RevokeAllSessions(ctx); err != nil {
		return db.Account{}, err
	}
	return s.store.UpdateAccountPassword(ctx, hash)
}

func (s *AuthService) UpdateProfile(ctx context.Context, username, displayName string) (db.Account, error) {
	if strings.TrimSpace(username) == "" || strings.TrimSpace(displayName) == "" {
		return db.Account{}, fmt.Errorf("%w: username and display name are required", ErrInvalidInput)
	}
	return s.store.UpdateAccount(ctx, username, displayName)
}

func principalFrom(account db.Account, sessionID uuid.UUID) Principal {
	return Principal{AccountID: account.ID, SessionID: sessionID, Username: account.Username, DisplayName: account.DisplayName}
}

func hashPassword(password string) (string, error) {
	if len(password) < 8 {
		return "", fmt.Errorf("%w: password must contain at least 8 characters", ErrInvalidInput)
	}
	return easyhash.Hash(password)
}

func newSessionToken() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	return token, sessionTokenHash(token), nil
}

func sessionTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
