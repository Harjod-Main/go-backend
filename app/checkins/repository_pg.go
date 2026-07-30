package checkins

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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

func (r *postgresRepo) HasRecentCheckIn(ctx context.Context, userID, placeID string, within time.Duration) (bool, error) {
	var exists bool
	since := time.Now().UTC().Add(-within)
	err := r.pool.QueryRow(ctx, `
SELECT EXISTS(
  SELECT 1 FROM check_ins
  WHERE user_id = $1::uuid
    AND place_id = $2::uuid
    AND created_at > $3
)`, userID, placeID, since).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check recent check-in: %w", err)
	}
	return exists, nil
}

const insertCheckInSQL = `
INSERT INTO check_ins (
  place_id, user_id, occupancy, satisfied, edit_suggestion, comment, points_awarded, created_at
) VALUES (
  $1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8
)
RETURNING check_in_id::text, created_at
`

const bumpCreditPointsSQL = `
UPDATE profiles
SET credit_points = credit_points + $2,
    updated_at = $3
WHERE user_id = $1::uuid
RETURNING credit_points
`

func (r *postgresRepo) Create(ctx context.Context, in CreateInput) (*CheckIn, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()
	out := &CheckIn{
		PlaceID:   in.PlaceID,
		UserID:    in.UserID,
		Occupancy: in.Occupancy,
		Satisfied: in.Satisfied,
		PointsAwarded: TotalPointsAwarded,
		PointsBreakdown: PointsBreakdown{
			CheckIn:   PointsCheckIn,
			Occupancy: PointsOccupancy,
		},
		EditSuggestion: in.EditSuggestion,
		Comment:        in.Comment,
		CreatedAt:      now,
	}

	err = tx.QueryRow(ctx, insertCheckInSQL,
		in.PlaceID,
		in.UserID,
		in.Occupancy,
		in.Satisfied,
		in.EditSuggestion,
		in.Comment,
		TotalPointsAwarded,
		now,
	).Scan(&out.CheckInID, &out.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert check-in: %w", err)
	}

	err = tx.QueryRow(ctx, bumpCreditPointsSQL, in.UserID, TotalPointsAwarded, now).Scan(&out.CreditPoints)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("bump credit points: profile missing for user %s", in.UserID)
		}
		return nil, fmt.Errorf("bump credit points: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit check-in: %w", err)
	}
	return out, nil
}

func NormalizeCreateRequest(body CreateCheckInRequest) (CreateInput, error) {
	if body.Satisfied == nil {
		return CreateInput{}, ErrInvalidInput
	}
	occupancy := strings.TrimSpace(strings.ToLower(body.Occupancy))
	if _, ok := validOccupancy[occupancy]; !ok {
		return CreateInput{}, ErrInvalidInput
	}

	satisfied := *body.Satisfied
	var edit *string
	var comment *string

	if satisfied {
		if body.EditSuggestion != nil && strings.TrimSpace(*body.EditSuggestion) != "" {
			return CreateInput{}, ErrInvalidInput
		}
		if body.Comment != nil && strings.TrimSpace(*body.Comment) != "" {
			return CreateInput{}, ErrInvalidInput
		}
	} else {
		if body.EditSuggestion == nil {
			return CreateInput{}, ErrInvalidInput
		}
		suggestion := strings.TrimSpace(strings.ToLower(*body.EditSuggestion))
		if _, ok := validEditSuggestions[suggestion]; !ok {
			return CreateInput{}, ErrInvalidInput
		}
		edit = &suggestion
		if body.Comment != nil {
			trimmed := strings.TrimSpace(*body.Comment)
			if len(trimmed) > MaxCommentLen {
				return CreateInput{}, ErrInvalidInput
			}
			if trimmed != "" {
				comment = &trimmed
			}
		}
	}

	return CreateInput{
		Occupancy:      occupancy,
		Satisfied:      satisfied,
		EditSuggestion: edit,
		Comment:        comment,
	}, nil
}
