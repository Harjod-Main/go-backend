package reports

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresRepo(pool *pgxpool.Pool) Repository {
	return &postgresRepo{pool: pool}
}

const insertIssueReportSQL = `
INSERT INTO issue_reports (user_id, category, description, photo_urls, reporter_email, status, created_at)
VALUES ($1::uuid, $2, $3, $4, $5, 'pending', $6)
RETURNING report_id::text, status::text
`

func (r *postgresRepo) CreateIssueReport(ctx context.Context, report *IssueReport) error {
	now := time.Now()
	report.CreatedAt = now
	photos := report.PhotoURLs
	if photos == nil {
		photos = []string{}
	}

	return r.pool.QueryRow(ctx, insertIssueReportSQL,
		report.UserID,
		report.Category,
		report.Description,
		photos,
		report.ReporterEmail,
		now,
	).Scan(&report.ReportID, &report.Status)
}

const insertReviewReportSQL = `
INSERT INTO review_reports (review_id, user_id, reason, detail, status, created_at)
VALUES ($1::uuid, $2::uuid, $3, $4, 'pending', $5)
RETURNING report_id::text, status::text
`

func (r *postgresRepo) CreateReviewReport(ctx context.Context, report *ReviewReport) error {
	now := time.Now()
	report.CreatedAt = now

	return r.pool.QueryRow(ctx, insertReviewReportSQL,
		report.ReviewID,
		report.UserID,
		report.Reason,
		report.Detail,
		now,
	).Scan(&report.ReportID, &report.Status)
}

const insertPlaceFeedbackSQL = `
INSERT INTO user_feedback (
	place_id, user_id, feedback_type, description, reporter_email,
	photo_url, photo_urls, old_value, suggested_value, status, created_at
)
VALUES (
	$1::uuid, $2::uuid, $3::feedback_type_enum, $4, $5,
	$6, $7, $8, $9, 'pending', $10
)
RETURNING feedback_id::text, status::text
`

func (r *postgresRepo) CreatePlaceFeedback(ctx context.Context, feedback *PlaceFeedback) error {
	now := time.Now()
	feedback.CreatedAt = now
	photos := feedback.PhotoURLs
	if photos == nil {
		photos = []string{}
	}

	return r.pool.QueryRow(ctx, insertPlaceFeedbackSQL,
		feedback.PlaceID,
		feedback.UserID,
		feedback.FeedbackType,
		feedback.Description,
		feedback.ReporterEmail,
		feedback.PhotoURL,
		photos,
		feedback.OldValue,
		feedback.SuggestedValue,
		now,
	).Scan(&feedback.FeedbackID, &feedback.Status)
}

func (r *postgresRepo) ReviewExists(ctx context.Context, reviewID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM reviews WHERE review_id = $1::uuid)`, reviewID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check review exists: %w", err)
	}
	return exists, nil
}

func (r *postgresRepo) PlaceExists(ctx context.Context, placeID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM places WHERE place_id = $1::uuid)`, placeID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check place exists: %w", err)
	}
	return exists, nil
}
