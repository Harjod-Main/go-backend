package reviews

import (
	"context"

	"github.com/RinTanth/go-backend/app/pagination"
)

type Repository interface {
	ListByPlace(ctx context.Context, placeID string, limit int, cursor *pagination.Cursor, viewerUserID string) ([]Review, *string, error)
	Create(ctx context.Context, review *Review) error
	Update(ctx context.Context, userID string, review *Review) error
	ReviewExists(ctx context.Context, reviewID string) (bool, error)
	SetReviewLiked(ctx context.Context, reviewID, userID string, liked bool) (likeCount int, err error)
}
