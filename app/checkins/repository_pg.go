package checkins

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/RinTanth/go-backend/app/pagination"
	"github.com/RinTanth/go-backend/app/points"
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
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM places
			WHERE place_id = $1::uuid
				AND COALESCE(is_blacklisted, false) = false
		)
	`, placeID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check place exists: %w", err)
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

const recentCheckInSQL = `
SELECT EXISTS(
  SELECT 1 FROM check_ins
  WHERE user_id = $1::uuid
    AND place_id = $2::uuid
    AND created_at > $3
)
`

func (r *postgresRepo) Create(ctx context.Context, in CreateInput) (*CheckIn, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Serialize concurrent check-ins for the same user+place so cooldown cannot
	// be bypassed by racing check-then-insert requests.
	if _, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext($1::text), hashtext($2::text))`,
		in.UserID, in.PlaceID,
	); err != nil {
		return nil, fmt.Errorf("advisory lock check-in: %w", err)
	}

	now := time.Now().UTC()
	var recent bool
	if err := tx.QueryRow(ctx, recentCheckInSQL, in.UserID, in.PlaceID, now.Add(-Cooldown)).Scan(&recent); err != nil {
		return nil, fmt.Errorf("check recent check-in: %w", err)
	}
	if recent {
		return nil, ErrCooldown
	}

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

	if err := points.InsertEvent(ctx, tx, points.Event{
		UserID:     in.UserID,
		Amount:     TotalPointsAwarded,
		Reason:     points.ReasonCheckIn,
		SourceType: "check_in",
		SourceID:   out.CheckInID,
		PlaceID:    &in.PlaceID,
		CreatedAt:  now,
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit check-in: %w", err)
	}
	return out, nil
}

const listByUserSQL = `
SELECT
  ci.check_in_id::text,
  ci.place_id::text,
  COALESCE(pl.name_th, ''),
  COALESCE(pl.name_en, ''),
  ci.points_awarded,
  ci.occupancy,
  ci.satisfied,
  ci.created_at
FROM check_ins ci
LEFT JOIN places pl ON pl.place_id = ci.place_id
WHERE ci.user_id = $1::uuid
  AND (
    $2::timestamptz IS NULL
    OR (ci.created_at, ci.check_in_id) < ($2::timestamptz, $3::uuid)
  )
ORDER BY ci.created_at DESC, ci.check_in_id DESC
LIMIT $4
`

func (r *postgresRepo) ListByUser(ctx context.Context, userID string, limit int, cursor *pagination.Cursor) ([]CheckInActivity, *string, error) {
	var cursorAt any
	var cursorID any
	if cursor != nil {
		cursorAt = cursor.CreatedAt
		cursorID = cursor.ID
	}

	rows, err := r.pool.Query(ctx, listByUserSQL, userID, cursorAt, cursorID, limit+1)
	if err != nil {
		return nil, nil, fmt.Errorf("list user check-ins: %w", err)
	}
	defer rows.Close()

	out := make([]CheckInActivity, 0, limit)
	for rows.Next() {
		var item CheckInActivity
		if err := rows.Scan(
			&item.CheckInID,
			&item.PlaceID,
			&item.PlaceNameTh,
			&item.PlaceNameEn,
			&item.PointsAwarded,
			&item.Occupancy,
			&item.Satisfied,
			&item.CreatedAt,
		); err != nil {
			return nil, nil, fmt.Errorf("scan user check-in: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate user check-ins: %w", err)
	}

	hasMore := len(out) > limit
	if hasMore {
		out = out[:limit]
	}
	var nextCursor *string
	if hasMore && len(out) > 0 {
		last := out[len(out)-1]
		nextCursor = pagination.NextFromLast(true, last.CreatedAt, last.CheckInID)
	}
	return out, nextCursor, nil
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
