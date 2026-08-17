package referrals

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/RinTanth/go-backend/app/points"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Keep in sync with app/profile username rules.
var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9._-]{3,30}$`)

type postgresRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresRepo(pool *pgxpool.Pool) Repository {
	return &postgresRepo{pool: pool}
}

const lookupReferrerSQL = `
SELECT user_id::text, display_name, username
FROM profiles
WHERE lower(username) = lower($1)
`

const lookupRefereeSQL = `
SELECT created_at
FROM profiles
WHERE user_id = $1::uuid
`

const lookupExistingSQL = `
SELECT referrer_user_id::text, referrer_points, referee_points
FROM referrals
WHERE referee_user_id = $1::uuid
`

const insertReferralSQL = `
INSERT INTO referrals (
  referrer_user_id, referee_user_id, invite_username, referrer_points, referee_points, created_at
) VALUES (
  $1::uuid, $2::uuid, $3, $4, $5, $6
)
RETURNING referral_id::text
`

const bumpCreditPointsSQL = `
UPDATE profiles
SET credit_points = credit_points + $2,
    updated_at = $3
WHERE user_id = $1::uuid
RETURNING credit_points
`

func (r *postgresRepo) Accept(ctx context.Context, in AcceptInput) (*AcceptOutcome, error) {
	username := strings.TrimSpace(in.InviteUsername)
	if !usernamePattern.MatchString(username) {
		return nil, ErrInvalidUsername
	}
	if strings.TrimSpace(in.RefereeUserID) == "" {
		return nil, ErrRefereeNotFound
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin referral tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1::text))`, in.RefereeUserID); err != nil {
		return nil, fmt.Errorf("lock referral accept: %w", err)
	}

	var referrerID, referrerDisplay, referrerUsername string
	err = tx.QueryRow(ctx, lookupReferrerSQL, username).Scan(&referrerID, &referrerDisplay, &referrerUsername)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrReferrerNotFound
		}
		return nil, fmt.Errorf("lookup referrer: %w", err)
	}
	if referrerID == in.RefereeUserID {
		return nil, ErrSelfReferral
	}

	var existingReferrer string
	var existingReferrerPts, existingRefereePts int
	err = tx.QueryRow(ctx, lookupExistingSQL, in.RefereeUserID).Scan(
		&existingReferrer, &existingReferrerPts, &existingRefereePts,
	)
	if err == nil {
		if existingReferrer != referrerID {
			return nil, ErrAlreadyReferred
		}
		return &AcceptOutcome{
			Created:             false,
			ReferrerUserID:      referrerID,
			ReferrerUsername:    referrerUsername,
			ReferrerDisplayName: referrerDisplay,
			RefereePoints:       existingRefereePts,
			ReferrerPoints:      existingReferrerPts,
		}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("lookup existing referral: %w", err)
	}

	var createdAt time.Time
	err = tx.QueryRow(ctx, lookupRefereeSQL, in.RefereeUserID).Scan(&createdAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRefereeNotFound
		}
		return nil, fmt.Errorf("lookup referee: %w", err)
	}
	if time.Since(createdAt) > MaxAccountAge {
		return nil, ErrNotEligible
	}

	now := time.Now().UTC()
	referrerPts := points.ReferralReferrer
	refereePts := points.ReferralReferee

	var referralID string
	err = tx.QueryRow(ctx, insertReferralSQL,
		referrerID, in.RefereeUserID, referrerUsername, referrerPts, refereePts, now,
	).Scan(&referralID)
	if err != nil {
		if isUniqueViolation(err) {
			err = tx.QueryRow(ctx, lookupExistingSQL, in.RefereeUserID).Scan(
				&existingReferrer, &existingReferrerPts, &existingRefereePts,
			)
			if err == nil {
				if existingReferrer != referrerID {
					return nil, ErrAlreadyReferred
				}
				return &AcceptOutcome{
					Created:             false,
					ReferrerUserID:      referrerID,
					ReferrerUsername:    referrerUsername,
					ReferrerDisplayName: referrerDisplay,
					RefereePoints:       existingRefereePts,
					ReferrerPoints:      existingReferrerPts,
				}, nil
			}
			return nil, ErrAlreadyReferred
		}
		return nil, fmt.Errorf("insert referral: %w", err)
	}

	if err := bumpPoints(ctx, tx, referrerID, referrerPts, now); err != nil {
		return nil, err
	}
	if err := bumpPoints(ctx, tx, in.RefereeUserID, refereePts, now); err != nil {
		return nil, err
	}
	if err := points.InsertEvent(ctx, tx, points.Event{
		UserID:     referrerID,
		Amount:     referrerPts,
		Reason:     points.ReasonReferral,
		SourceType: "referral",
		SourceID:   referralID + ":referrer",
		CreatedAt:  now,
	}); err != nil {
		return nil, err
	}
	if err := points.InsertEvent(ctx, tx, points.Event{
		UserID:     in.RefereeUserID,
		Amount:     refereePts,
		Reason:     points.ReasonReferral,
		SourceType: "referral",
		SourceID:   referralID + ":referee",
		CreatedAt:  now,
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit referral: %w", err)
	}

	return &AcceptOutcome{
		Created:             true,
		ReferrerUserID:      referrerID,
		ReferrerUsername:    referrerUsername,
		ReferrerDisplayName: referrerDisplay,
		RefereePoints:       refereePts,
		ReferrerPoints:      referrerPts,
		CreatedAt:           now,
	}, nil
}

func bumpPoints(ctx context.Context, tx pgx.Tx, userID string, amount int, now time.Time) error {
	var total int
	err := tx.QueryRow(ctx, bumpCreditPointsSQL, userID, amount, now).Scan(&total)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("bump credit points: profile missing for user %s", userID)
		}
		return fmt.Errorf("bump credit points: %w", err)
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
