package db

import (
	"context"
	"database/sql"
	"strings"

	"github.com/google/uuid"
)

const credentialSelect = `
	SELECT c.*, p.code AS provider_code
	FROM provider_credentials c JOIN model_providers p ON p.id=c.provider_id`

func (s *Store) ListProviderCredentials(ctx context.Context, providerID *uuid.UUID) ([]ProviderCredential, error) {
	return collectRows[ProviderCredential](s.pool.Query(ctx, credentialSelect+` WHERE ($1 IS NULL OR c.provider_id=$1) ORDER BY p.code,c.is_active DESC,c.created_at DESC`, providerID))
}

func (s *Store) GetProviderCredential(ctx context.Context, id uuid.UUID) (ProviderCredential, error) {
	return one[ProviderCredential](s.pool.Query(ctx, credentialSelect+` WHERE c.id=$1`, id))
}

func (s *Store) GetActiveProviderCredential(ctx context.Context, providerID uuid.UUID) (ProviderCredential, error) {
	return one[ProviderCredential](s.pool.Query(ctx, credentialSelect+` WHERE c.provider_id=$1 AND c.is_active=TRUE`, providerID))
}

type SaveCredentialInput struct {
	ID              uuid.UUID
	ProviderID      uuid.UUID
	Name            string
	EncryptedAPIKey string
	KeyHint         string
	Activate        bool
}

func (s *Store) CreateProviderCredential(ctx context.Context, input SaveCredentialInput) (ProviderCredential, error) {
	var credential ProviderCredential
	err := withTx(ctx, s.pool, func(tx *Tx) error {
		activate := input.Activate
		if !activate {
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM provider_credentials WHERE provider_id=$1 AND is_active)`, input.ProviderID).Scan(&exists); err != nil {
				return err
			}
			activate = !exists
		}
		if activate {
			if _, err := tx.Exec(ctx, `UPDATE provider_credentials SET is_active=0 WHERE provider_id=$1`, input.ProviderID); err != nil {
				return err
			}
		}
		id := input.ID
		if id == uuid.Nil {
			id = uuid.New()
		}
		if _, err := tx.Exec(ctx, `INSERT INTO provider_credentials (id,provider_id,name,encrypted_api_key,key_hint,is_active) VALUES ($1,$2,$3,$4,$5,$6)`, id, input.ProviderID, strings.TrimSpace(input.Name), input.EncryptedAPIKey, input.KeyHint, activate); err != nil {
			return err
		}
		var err error
		credential, err = one[ProviderCredential](tx.Query(ctx, credentialSelect+` WHERE c.id=$1`, id))
		return err
	})
	return credential, err
}

func (s *Store) UpdateProviderCredential(ctx context.Context, input SaveCredentialInput) (ProviderCredential, error) {
	var credential ProviderCredential
	err := withTx(ctx, s.pool, func(tx *Tx) error {
		if input.Activate {
			if _, err := tx.Exec(ctx, `UPDATE provider_credentials SET is_active=0 WHERE provider_id=$1`, input.ProviderID); err != nil {
				return err
			}
		}
		result, err := tx.Exec(ctx, `UPDATE provider_credentials SET provider_id=$2,name=$3,encrypted_api_key=$4,key_hint=$5,is_active=CASE WHEN $6 THEN 1 ELSE is_active END,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=$1`, input.ID, input.ProviderID, strings.TrimSpace(input.Name), input.EncryptedAPIKey, input.KeyHint, input.Activate)
		if err != nil {
			return err
		}
		if changed, _ := result.RowsAffected(); changed == 0 {
			return ErrNotFound
		}
		credential, err = one[ProviderCredential](tx.Query(ctx, credentialSelect+` WHERE c.id=$1`, input.ID))
		return err
	})
	return credential, err
}

func (s *Store) ActivateProviderCredential(ctx context.Context, id uuid.UUID) (ProviderCredential, error) {
	var credential ProviderCredential
	err := withTx(ctx, s.pool, func(tx *Tx) error {
		var providerID uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT provider_id FROM provider_credentials WHERE id=$1`, id).Scan(&providerID); err != nil {
			if err == sql.ErrNoRows {
				return ErrNotFound
			}
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE provider_credentials SET is_active=0 WHERE provider_id=$1`, providerID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE provider_credentials SET is_active=1,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=$1`, id); err != nil {
			return err
		}
		var err error
		credential, err = one[ProviderCredential](tx.Query(ctx, credentialSelect+` WHERE c.id=$1`, id))
		return err
	})
	return credential, err
}

func (s *Store) DeleteProviderCredential(ctx context.Context, id uuid.UUID) error {
	r, err := s.pool.Exec(ctx, `DELETE FROM provider_credentials WHERE id=$1`, id)
	if err == nil {
		if changed, _ := r.RowsAffected(); changed == 0 {
			return ErrNotFound
		}
	}
	return err
}

func (s *Store) TouchProviderCredential(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE provider_credentials SET last_used_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=$1`, id)
	return err
}
