package reviews

import "context"

type Repository interface {
	ListByPlace(ctx context.Context, placeID string, limit int, offset int) ([]Review, int, error)
	Create(ctx context.Context, review *Review) error
	// Update mutates an owned review. Returns ErrNotFound when the review is
	// missing or does not belong to userID.
	Update(ctx context.Context, userID string, review *Review) error
}
