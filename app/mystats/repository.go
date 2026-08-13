package mystats

import "context"

type Stats struct {
	ReviewCount          int `json:"reviewCount"`
	PlaceSubmissionCount int `json:"placeSubmissionCount"`
	IssueReportCount     int `json:"issueReportCount"`
}

type Repository interface {
	CountByUser(ctx context.Context, userID string) (*Stats, error)
}
