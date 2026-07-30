package reports

import "context"

type Repository interface {
	CreateIssueReport(ctx context.Context, report *IssueReport) error
	CreateReviewReport(ctx context.Context, report *ReviewReport) error
	CreatePlaceFeedback(ctx context.Context, feedback *PlaceFeedback) error
	ListIssueReportsByUser(ctx context.Context, userID string, limit, offset int) ([]IssueReport, int, error)
	ReviewExists(ctx context.Context, reviewID string) (bool, error)
	PlaceExists(ctx context.Context, placeID string) (bool, error)
}
