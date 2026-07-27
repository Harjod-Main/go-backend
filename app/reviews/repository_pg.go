package reviews

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresRepo(pool *pgxpool.Pool) Repository {
	return &postgresRepo{pool: pool}
}

const listByPlaceSQL = `
SELECT
	review_id::text,
	place_id::text,
	user_id::text,
	display_name,
	rating,
	description,
	photo_urls,
	created_at
FROM reviews
WHERE place_id = $1::uuid
ORDER BY created_at DESC
LIMIT $2 OFFSET $3
`

const countByPlaceSQL = `SELECT count(*) FROM reviews WHERE place_id = $1::uuid`

func (r *postgresRepo) ListByPlace(ctx context.Context, placeID string, limit int, offset int) ([]Review, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, countByPlaceSQL, placeID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count reviews: %w", err)
	}

	rows, err := r.pool.Query(ctx, listByPlaceSQL, placeID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list reviews: %w", err)
	}
	defer rows.Close()

	var reviews []Review
	for rows.Next() {
		var rv Review
		var photoURLs []string
		if err := rows.Scan(
			&rv.ReviewID,
			&rv.PlaceID,
			&rv.UserID,
			&rv.DisplayName,
			&rv.Rating,
			&rv.Description,
			&photoURLs,
			&rv.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan review: %w", err)
		}
		rv.PhotoURLs = photoURLs
		reviews = append(reviews, rv)
	}
	if reviews == nil {
		reviews = []Review{}
	}
	return reviews, total, nil
}

const insertReviewSQL = `
INSERT INTO reviews (place_id, user_id, display_name, rating, description, photo_urls, created_at, updated_at)
VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $7)
RETURNING review_id::text
`

func (r *postgresRepo) Create(ctx context.Context, review *Review) error {
	now := time.Now()
	review.CreatedAt = now

	photos := review.PhotoURLs
	if photos == nil {
		photos = []string{}
	}

	return r.pool.QueryRow(ctx, insertReviewSQL,
		review.PlaceID,
		review.UserID,
		review.DisplayName,
		review.Rating,
		review.Description,
		photos,
		now,
	).Scan(&review.ReviewID)
}
