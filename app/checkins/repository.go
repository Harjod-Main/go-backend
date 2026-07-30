package checkins

import "context"

type Repository interface {
	PlaceExists(ctx context.Context, placeID string) (bool, error)
	// Create inserts a check-in and awards points. Concurrent creates for the
	// same user+place are serialized; returns ErrCooldown when still within Cooldown.
	Create(ctx context.Context, in CreateInput) (*CheckIn, error)
	ListByUser(ctx context.Context, userID string, limit, offset int) ([]CheckInActivity, int, error)
}
