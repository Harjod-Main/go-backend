package places

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const placeReactionCountsSQL = `
SELECT
  COALESCE(SUM(CASE WHEN reaction = 'like' THEN 1 ELSE 0 END), 0)::int,
  COALESCE(SUM(CASE WHEN reaction = 'unlike' THEN 1 ELSE 0 END), 0)::int
FROM place_reactions
WHERE place_id = $1::uuid
`

const myPlaceReactionSQL = `
SELECT reaction
FROM place_reactions
WHERE place_id = $1::uuid AND user_id = $2::uuid
`

func (r *postgresRepo) loadPlaceReaction(ctx context.Context, placeID, userID string) (*PlaceReactionResponse, error) {
	out := &PlaceReactionResponse{PlaceID: placeID}
	if err := r.pool.QueryRow(ctx, placeReactionCountsSQL, placeID).Scan(&out.LikeCount, &out.UnlikeCount); err != nil {
		return nil, fmt.Errorf("count place reactions: %w", err)
	}
	if strings.TrimSpace(userID) != "" {
		var reaction string
		err := r.pool.QueryRow(ctx, myPlaceReactionSQL, placeID, userID).Scan(&reaction)
		if err == nil {
			kind := PlaceReactionKind(reaction)
			out.MyReaction = &kind
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get my place reaction: %w", err)
		}
	}
	return out, nil
}

func (r *postgresRepo) GetPlaceReaction(ctx context.Context, placeID, userID string) (*PlaceReactionResponse, error) {
	return r.loadPlaceReaction(ctx, placeID, userID)
}

const upsertPlaceReactionSQL = `
INSERT INTO place_reactions (place_id, user_id, reaction, created_at, updated_at)
VALUES ($1::uuid, $2::uuid, $3, $4, $4)
ON CONFLICT (place_id, user_id) DO UPDATE
SET reaction = EXCLUDED.reaction,
    updated_at = EXCLUDED.updated_at
`

func (r *postgresRepo) SetPlaceReaction(ctx context.Context, placeID, userID string, reaction PlaceReactionKind) (*PlaceReactionResponse, error) {
	now := time.Now().UTC()
	if _, err := r.pool.Exec(ctx, upsertPlaceReactionSQL, placeID, userID, string(reaction), now); err != nil {
		return nil, fmt.Errorf("set place reaction: %w", err)
	}
	return r.loadPlaceReaction(ctx, placeID, userID)
}

const clearPlaceReactionSQL = `
DELETE FROM place_reactions
WHERE place_id = $1::uuid AND user_id = $2::uuid
`

func (r *postgresRepo) ClearPlaceReaction(ctx context.Context, placeID, userID string) (*PlaceReactionResponse, error) {
	if _, err := r.pool.Exec(ctx, clearPlaceReactionSQL, placeID, userID); err != nil {
		return nil, fmt.Errorf("clear place reaction: %w", err)
	}
	return r.loadPlaceReaction(ctx, placeID, userID)
}
