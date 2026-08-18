package points

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

const bumpCreditPointsSQL = `
UPDATE profiles
SET credit_points = credit_points + $2,
    updated_at = $3
WHERE user_id = $1::uuid
RETURNING credit_points
`

// Award increments profile credit_points and writes a ledger event on tx.
func Award(ctx context.Context, tx pgx.Tx, event Event) error {
	if tx == nil {
		return fmt.Errorf("award points: missing tx")
	}
	now := event.CreatedAt
	if now.IsZero() {
		now = time.Now().UTC()
		event.CreatedAt = now
	}

	var total int
	err := tx.QueryRow(ctx, bumpCreditPointsSQL, event.UserID, event.Amount, now).Scan(&total)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("award points: profile missing for user %s", event.UserID)
		}
		return fmt.Errorf("award points: %w", err)
	}
	return InsertEvent(ctx, tx, event)
}
