package reviews

import (
	"context"

	"github.com/RinTanth/go-backend/app/pagination"
)

type Repository interface {
	ListByPlace(ctx context.Context, placeID string, limit int, cursor *pagination.Cursor) ([]Review, *string, error)
	Create(ctx context.Context, review *Review) error
	Update(ctx context.Context, userID string, review *Review) error
}
