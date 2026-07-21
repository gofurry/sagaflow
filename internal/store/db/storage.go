package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

func (s *Store) CreateLocalObject(ctx context.Context, object LocalObject) (LocalObject, error) {
	if object.ID == uuid.Nil {
		object.ID = uuid.New()
	}
	if object.State == "" {
		object.State = "pending"
	}
	return one[LocalObject](s.pool.Query(ctx, `
		INSERT INTO local_objects (id,project_id,object_key,original_name,purpose,mime_type,size_bytes,sha256,state)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING *`, object.ID, object.ProjectID, object.ObjectKey,
		object.OriginalName, object.Purpose, object.MimeType, object.SizeBytes, object.SHA256, object.State))
}

func (s *Store) MarkLocalObjectReady(ctx context.Context, id uuid.UUID, key, sha256 string, size int64) (LocalObject, error) {
	return one[LocalObject](s.pool.Query(ctx, `
		UPDATE local_objects SET object_key=$2,sha256=$3,size_bytes=$4,state='ready',updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE id=$1 RETURNING *`, id, key, sha256, size))
}

func (s *Store) MarkLocalObjectFailed(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE local_objects SET state='failed',updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=$1`, id)
	return err
}

func (s *Store) GetLocalObject(ctx context.Context, id uuid.UUID) (LocalObject, error) {
	return one[LocalObject](s.pool.Query(ctx, `SELECT * FROM local_objects WHERE id=$1 AND state<>'deleted'`, id))
}

func (s *Store) MarkLocalObjectDeleting(ctx context.Context, id uuid.UUID) (LocalObject, error) {
	return one[LocalObject](s.pool.Query(ctx, `
		UPDATE local_objects SET state='deleting',updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE id=$1 AND state<>'deleted' RETURNING *`, id))
}

func (s *Store) MarkLocalObjectDeleted(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE local_objects SET state='deleted',updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=$1`, id)
	return err
}

type SaveS3ConnectionInput struct {
	ID                   uuid.UUID
	Name                 string
	Provider             string
	Endpoint             string
	PublicEndpoint       string
	Region               string
	Bucket               string
	Prefix               string
	ForcePathStyle       bool
	EncryptedCredentials string
	Enabled              bool
	IsDefault            bool
}

func (s *Store) ListS3Connections(ctx context.Context) ([]S3Connection, error) {
	return collectRows[S3Connection](s.pool.Query(ctx, `SELECT * FROM s3_connections ORDER BY is_default DESC,name`))
}

func (s *Store) GetS3Connection(ctx context.Context, id uuid.UUID) (S3Connection, error) {
	return one[S3Connection](s.pool.Query(ctx, `SELECT * FROM s3_connections WHERE id=$1`, id))
}

func (s *Store) SaveS3Connection(ctx context.Context, input SaveS3ConnectionInput) (S3Connection, error) {
	if input.ID == uuid.Nil {
		input.ID = uuid.New()
	}
	if input.Provider == "" {
		input.Provider = "s3"
	}
	if input.Prefix == "" {
		input.Prefix = "sagaflow"
	}
	var saved S3Connection
	err := withTx(ctx, s.pool, func(tx *Tx) error {
		if input.IsDefault {
			if _, err := tx.Exec(ctx, `UPDATE s3_connections SET is_default=0`); err != nil {
				return err
			}
		}
		var err error
		saved, err = one[S3Connection](tx.Query(ctx, `
			INSERT INTO s3_connections (id,name,provider,endpoint,public_endpoint,region,bucket,prefix,force_path_style,encrypted_credentials,enabled,is_default)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
			ON CONFLICT(id) DO UPDATE SET name=excluded.name,provider=excluded.provider,endpoint=excluded.endpoint,
			public_endpoint=excluded.public_endpoint,region=excluded.region,bucket=excluded.bucket,prefix=excluded.prefix,
			force_path_style=excluded.force_path_style,encrypted_credentials=excluded.encrypted_credentials,
			enabled=excluded.enabled,is_default=excluded.is_default,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
			RETURNING *`, input.ID, strings.TrimSpace(input.Name), input.Provider, strings.TrimRight(strings.TrimSpace(input.Endpoint), "/"),
			strings.TrimRight(strings.TrimSpace(input.PublicEndpoint), "/"), input.Region, input.Bucket, strings.Trim(input.Prefix, "/"),
			input.ForcePathStyle, input.EncryptedCredentials, input.Enabled, input.IsDefault))
		return err
	})
	return saved, err
}

func (s *Store) DeleteS3Connection(ctx context.Context, id uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM s3_connections WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("%w: 该连接仍有远端发布记录", ErrConflict)
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CreateAssetRemoteExport(ctx context.Context, item AssetRemoteExport) (AssetRemoteExport, error) {
	if item.ID == uuid.Nil {
		item.ID = uuid.New()
	}
	if item.State == "" {
		item.State = "ready"
	}
	return one[AssetRemoteExport](s.pool.Query(ctx, `
		INSERT INTO asset_remote_exports (id,asset_id,connection_id,object_key,etag,public_url,state)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING *`, item.ID, item.AssetID, item.ConnectionID, item.ObjectKey, item.ETag, item.PublicURL, item.State))
}

func (s *Store) ListAssetRemoteExports(ctx context.Context, assetID uuid.UUID) ([]AssetRemoteExport, error) {
	return collectRows[AssetRemoteExport](s.pool.Query(ctx, `SELECT * FROM asset_remote_exports WHERE asset_id=$1 ORDER BY created_at DESC`, assetID))
}

func (s *Store) GetAssetRemoteExport(ctx context.Context, id uuid.UUID) (AssetRemoteExport, error) {
	return one[AssetRemoteExport](s.pool.Query(ctx, `SELECT * FROM asset_remote_exports WHERE id=$1`, id))
}

func (s *Store) DeleteAssetRemoteExport(ctx context.Context, id uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM asset_remote_exports WHERE id=$1`, id)
	if err == nil {
		if changed, _ := result.RowsAffected(); changed == 0 {
			return ErrNotFound
		}
	}
	return err
}
