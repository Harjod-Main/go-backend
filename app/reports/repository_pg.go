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

const listIssueReportsByUserSQL = `
SELECT
  report_id::text,
  user_id::text,
  category,
  description,
  COALESCE(photo_urls, '{}'::text[]),
  reporter_email,
  status::text,
  created_at
FROM issue_reports
WHERE user_id = $1::uuid
ORDER BY created_at DESC
LIMIT $2 OFFSET $3
`

const countIssueReportsByUserSQL = `
SELECT COUNT(*)::int
FROM issue_reports
WHERE user_id = $1::uuid
`

func (r *postgresRepo) ListIssueReportsByUser(ctx context.Context, userID string, limit, offset int) ([]IssueReport, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, countIssueReportsByUserSQL, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count issue reports: %w", err)
	}

	rows, err := r.pool.Query(ctx, listIssueReportsByUserSQL, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list issue reports: %w", err)
	}
	defer rows.Close()

	out := make([]IssueReport, 0, limit)
	for rows.Next() {
		var item IssueReport
		var userIDVal *string
		if err := rows.Scan(
			&item.ReportID,
			&userIDVal,
			&item.Category,
			&item.Description,
			&item.PhotoURLs,
			&item.ReporterEmail,
			&item.Status,
			&item.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan issue report: %w", err)
		}
		item.UserID = userIDVal
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate issue reports: %w", err)
	}
	return out, total, nil
}
