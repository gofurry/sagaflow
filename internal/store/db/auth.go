package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (s *Store) AuthInitialized(ctx context.Context) (bool, error) {
	var count int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM account`).Scan(&count)
	return count > 0, err
}

func (s *Store) CreateAccount(ctx context.Context, username, displayName, passwordHash string) (Account, error) {
	if initialized, err := s.AuthInitialized(ctx); err != nil {
		return Account{}, err
	} else if initialized {
		return Account{}, ErrConflict
	}
	id := uuid.New()
	return one[Account](s.pool.Query(ctx, `
		INSERT INTO account (id, username, display_name, password_hash)
		VALUES ($1,$2,$3,$4)
		RETURNING *`, id, strings.TrimSpace(username), strings.TrimSpace(displayName), passwordHash))
}

func (s *Store) GetAccount(ctx context.Context) (Account, error) {
	return one[Account](s.pool.Query(ctx, `SELECT * FROM account LIMIT 1`))
}

func (s *Store) GetAccountByUsername(ctx context.Context, username string) (Account, error) {
	return one[Account](s.pool.Query(ctx, `SELECT * FROM account WHERE username = $1 COLLATE NOCASE`, strings.TrimSpace(username)))
}

func (s *Store) UpdateAccount(ctx context.Context, username, displayName string) (Account, error) {
	return one[Account](s.pool.Query(ctx, `
		UPDATE account SET username=$1, display_name=$2, updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
		RETURNING *`, strings.TrimSpace(username), strings.TrimSpace(displayName)))
}

func (s *Store) UpdateAccountPassword(ctx context.Context, passwordHash string) (Account, error) {
	return one[Account](s.pool.Query(ctx, `
		UPDATE account SET password_hash=$1, updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
		RETURNING *`, passwordHash))
}

func (s *Store) TouchAccountLogin(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `UPDATE account SET last_login_at=strftime('%Y-%m-%dT%H:%M:%fZ','now'), updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')`)
	return err
}

func (s *Store) CreateSession(ctx context.Context, tokenHash string, expiresAt time.Time) (Session, error) {
	return one[Session](s.pool.Query(ctx, `
		INSERT INTO sessions (id, token_hash, expires_at) VALUES ($1,$2,$3)
		RETURNING *`, uuid.New(), tokenHash, expiresAt.UTC().Format(time.RFC3339Nano)))
}

func (s *Store) GetActiveSessionByTokenHash(ctx context.Context, tokenHash string) (Session, error) {
	return one[Session](s.pool.Query(ctx, `
		UPDATE sessions SET last_seen_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE token_hash=$1 AND revoked_at IS NULL AND expires_at > strftime('%Y-%m-%dT%H:%M:%fZ','now')
		RETURNING *`, tokenHash))
}

func (s *Store) RevokeSession(ctx context.Context, id uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `UPDATE sessions SET revoked_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=$1 AND revoked_at IS NULL`, id)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) RevokeAllSessions(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `UPDATE sessions SET revoked_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE revoked_at IS NULL`)
	return err
}

func mapSQLError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
