package reviews

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/RinTanth/go-backend/app/pagination"
	"github.com/jackc/pgx/v5"
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
	r.review_id::text,
	r.place_id::text,
	r.user_id::text,
	r.display_name,
	r.rating,
	r.description,
	r.photo_urls,
	r.created_at,
	COALESCE(lc.like_count, 0)::int AS like_count,
	CASE
		WHEN $5::uuid IS NULL THEN false
		ELSE EXISTS (
			SELECT 1 FROM review_likes rl
			WHERE rl.review_id = r.review_id AND rl.user_id = $5::uuid
		)
	END AS liked_by_me
FROM reviews r
LEFT JOIN (
	SELECT review_id, COUNT(*)::int AS like_count
	FROM review_likes
	GROUP BY review_id
) lc ON lc.review_id = r.review_id
WHERE r.place_id = $1::uuid
  AND (
    $2::timestamptz IS NULL
    OR (r.created_at, r.review_id) < ($2::timestamptz, $3::uuid)
  )
ORDER BY r.created_at DESC, r.review_id DESC
LIMIT $4
`

func (r *postgresRepo) ListByPlace(ctx context.Context, placeID string, limit int, cursor *pagination.Cursor, viewerUserID string) ([]Review, *string, error) {
	var cursorAt any
	var cursorID any
	if cursor != nil {
		cursorAt = cursor.CreatedAt
		cursorID = cursor.ID
	}
	var viewer any
	if strings.TrimSpace(viewerUserID) != "" {
		viewer = viewerUserID
	}

	rows, err := r.pool.Query(ctx, listByPlaceSQL, placeID, cursorAt, cursorID, limit+1, viewer)
	if err != nil {
		return nil, nil, fmt.Errorf("list reviews: %w", err)
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
			&rv.LikeCount,
			&rv.LikedByMe,
		); err != nil {
			return nil, nil, fmt.Errorf("scan review: %w", err)
		}
		rv.PhotoURLs = photoURLs
		reviews = append(reviews, rv)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate reviews: %w", err)
	}
	if reviews == nil {
		reviews = []Review{}
	}

	hasMore := len(reviews) > limit
	if hasMore {
		reviews = reviews[:limit]
	}
	var nextCursor *string
	if hasMore && len(reviews) > 0 {
		last := reviews[len(reviews)-1]
		nextCursor = pagination.NextFromLast(true, last.CreatedAt, last.ReviewID)
	}
	return reviews, nextCursor, nil
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

const updateReviewSQL = `
UPDATE reviews
SET rating = $1,
    description = $2,
    photo_urls = $3,
    updated_at = $4
WHERE review_id = $5::uuid
  AND user_id = $6::uuid
RETURNING place_id::text, display_name, created_at
`

func (r *postgresRepo) Update(ctx context.Context, userID string, review *Review) error {
	now := time.Now()
	photos := review.PhotoURLs
	if photos == nil {
		photos = []string{}
	}

	err := r.pool.QueryRow(ctx, updateReviewSQL,
		review.Rating,
		review.Description,
		photos,
		now,
		review.ReviewID,
		userID,
	).Scan(&review.PlaceID, &review.DisplayName, &review.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("update review: %w", err)
	}
	review.UserID = userID
	return nil
}

func (r *postgresRepo) ReviewExists(ctx context.Context, reviewID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM reviews WHERE review_id = $1::uuid)`, reviewID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check review exists: %w", err)
	}
	return exists, nil
}

const insertReviewLikeSQL = `
INSERT INTO review_likes (review_id, user_id, created_at)
VALUES ($1::uuid, $2::uuid, $3)
ON CONFLICT (review_id, user_id) DO NOTHING
`

const deleteReviewLikeSQL = `
DELETE FROM review_likes
WHERE review_id = $1::uuid AND user_id = $2::uuid
`

const countReviewLikesSQL = `
SELECT COUNT(*)::int FROM review_likes WHERE review_id = $1::uuid
`

func (r *postgresRepo) SetReviewLiked(ctx context.Context, reviewID, userID string, liked bool) (int, error) {
	now := time.Now().UTC()
	if liked {
		if _, err := r.pool.Exec(ctx, insertReviewLikeSQL, reviewID, userID, now); err != nil {
			return 0, fmt.Errorf("insert review like: %w", err)
		}
	} else {
		if _, err := r.pool.Exec(ctx, deleteReviewLikeSQL, reviewID, userID); err != nil {
			return 0, fmt.Errorf("delete review like: %w", err)
		}
	}
	var count int
	if err := r.pool.QueryRow(ctx, countReviewLikesSQL, reviewID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count review likes: %w", err)
	}
	return count, nil
}
