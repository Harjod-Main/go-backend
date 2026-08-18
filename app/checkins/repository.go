package checkins

import (
	"context"

	"github.com/RinTanth/go-backend/app/pagination"
)

type Repository interface {
	// PlaceExists is true for a visible (non-blacklisted) place.
	PlaceExists(ctx context.Context, placeID string) (bool, error)
	// Create inserts a check-in and awards points. Concurrent creates for the
	// same user+place are serialized; returns ErrCooldown when still within Cooldown.
	Create(ctx context.Context, in CreateInput) (*CheckIn, error)
	ListByUser(ctx context.Context, userID string, limit int, cursor *pagination.Cursor) ([]CheckInActivity, *string, error)
}
