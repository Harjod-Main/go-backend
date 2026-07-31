package reports

import (
	"context"

	"github.com/RinTanth/go-backend/app/pagination"
)

type Repository interface {
	CreateIssueReport(ctx context.Context, report *IssueReport) error
	CreateReviewReport(ctx context.Context, report *ReviewReport) error
	CreatePlaceFeedback(ctx context.Context, feedback *PlaceFeedback) error
	ListIssueReportsByUser(ctx context.Context, userID string, limit int, cursor *pagination.Cursor) ([]IssueReport, *string, error)
	ReviewExists(ctx context.Context, reviewID string) (bool, error)
	PlaceExists(ctx context.Context, placeID string) (bool, error)
}
