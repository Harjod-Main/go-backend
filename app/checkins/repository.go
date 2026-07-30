package checkins

import (
	"context"
	"time"
)

type Repository interface {
	PlaceExists(ctx context.Context, placeID string) (bool, error)
	HasRecentCheckIn(ctx context.Context, userID, placeID string, within time.Duration) (bool, error)
	Create(ctx context.Context, in CreateInput) (*CheckIn, error)
}
