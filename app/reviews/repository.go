package reviews

import "context"

type Repository interface {
	ListByPlace(ctx context.Context, placeID string, limit int, offset int) ([]Review, int, error)
	Create(ctx context.Context, review *Review) error
}
