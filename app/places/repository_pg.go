package places

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresRepo(pool *pgxpool.Pool) Repository {
	return &postgresRepo{pool: pool}
}

func (r *postgresRepo) PlaceExists(ctx context.Context, placeID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM places WHERE place_id = $1::uuid)`, placeID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check place exists: %w", err)
	}
	return exists, nil
}

func scanJSON[T any](ctx context.Context, pool *pgxpool.Pool, sql string, id string, label string) (*T, error) {
	var raw []byte
	err := pool.QueryRow(ctx, sql, id).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get %s: %w", label, err)
	}
	if raw == nil || string(raw) == "null" {
		return nil, nil
	}

	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode %s: %w", label, err)
	}
	return &value, nil
}

const deleteEntityImagesSQL = `
DELETE FROM place_images
WHERE entity_type = $1 AND entity_id = $2
`

const insertPrivilegeImageSQL = `
INSERT INTO place_images (
	entity_type, entity_id, storage_path, is_primary, is_verified, uploaded_by
) VALUES (
	$1, $2, $3, $4, false, $5::uuid
)
`

func replaceEntityImages(
	ctx context.Context,
	tx pgx.Tx,
	entityType string,
	entityID string,
	urls []string,
	uploadedBy string,
) error {
	if _, err := tx.Exec(ctx, deleteEntityImagesSQL, entityType, entityID); err != nil {
		return fmt.Errorf("delete %s images: %w", entityType, err)
	}
	for i, photoURL := range urls {
		trimmed := strings.TrimSpace(photoURL)
		if trimmed == "" {
			continue
		}
		if _, err := tx.Exec(ctx, insertPrivilegeImageSQL,
			entityType,
			entityID,
			trimmed,
			i == 0,
			uploadedBy,
		); err != nil {
			return fmt.Errorf("insert %s image: %w", entityType, err)
		}
	}
	return nil
}
