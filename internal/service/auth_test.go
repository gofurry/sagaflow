package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofurry/sagaflow/internal/config"
	"github.com/gofurry/sagaflow/internal/platform/sqlite"
	"github.com/gofurry/sagaflow/internal/store/db"
)

func TestSingleAccountAuthenticationLifecycle(t *testing.T) {
	ctx := context.Background()
	database, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "sagaflow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	auth, err := NewAuthService(db.New(database), config.AuthConfig{SessionTTL: time.Hour.String(), CookieName: "session"})
	if err != nil {
		t.Fatal(err)
	}
	account, err := auth.Initialize(ctx, "creator", "创作者", "password-123")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Initialize(ctx, "other", "Other", "password-456"); err == nil {
		t.Fatal("expected a second account to be rejected")
	}
	token, _, principal, err := auth.Login(ctx, "creator", "password-123")
	if err != nil {
		t.Fatal(err)
	}
	if principal.AccountID != account.ID {
		t.Fatalf("unexpected principal account: %s", principal.AccountID)
	}
	if authenticated, err := auth.AuthenticateToken(ctx, token); err != nil || authenticated.AccountID != account.ID {
		t.Fatalf("authenticate token: principal=%+v err=%v", authenticated, err)
	}
	if err := auth.Logout(ctx, principal); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.AuthenticateToken(ctx, token); err == nil {
		t.Fatal("expected the revoked session to be rejected")
	}
}
