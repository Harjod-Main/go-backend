package points

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	ReasonCheckIn          = "check_in"
	ReasonReview           = "review"
	ReasonPlaceSubmission  = "place_submission"
	ReasonCorrection       = "correction"
	ReasonReferral         = "referral"
)

type Event struct {
	UserID     string
	Amount     int
	Reason     string
	SourceType string
	SourceID   string
	PlaceID    *string
	CreatedAt  time.Time
}

type execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

const insertEventSQL = `
INSERT INTO credit_point_events (
  user_id, amount, reason, source_type, source_id, place_id, created_at
) VALUES (
  $1::uuid, $2, $3, $4, $5, $6::uuid, $7
)
ON CONFLICT (user_id, reason, source_type, source_id) DO NOTHING
`

func InsertEvent(ctx context.Context, exec execer, event Event) error {
	if exec == nil {
		return fmt.Errorf("insert credit event: missing execer")
	}
	if event.Amount <= 0 {
		return fmt.Errorf("insert credit event: amount must be positive")
	}
	reason := strings.TrimSpace(event.Reason)
	sourceType := strings.TrimSpace(event.SourceType)
	sourceID := strings.TrimSpace(event.SourceID)
	if reason == "" || sourceType == "" || sourceID == "" {
		return fmt.Errorf("insert credit event: reason, source type, and source id are required")
	}
	createdAt := event.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	var placeID any
	if event.PlaceID != nil && strings.TrimSpace(*event.PlaceID) != "" {
		placeID = strings.TrimSpace(*event.PlaceID)
	}
	if _, err := exec.Exec(ctx, insertEventSQL,
		event.UserID,
		event.Amount,
		reason,
		sourceType,
		sourceID,
		placeID,
		createdAt,
	); err != nil {
		return fmt.Errorf("insert credit event: %w", err)
	}
	return nil
}
