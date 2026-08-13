package mystats

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresRepo(pool *pgxpool.Pool) Repository {
	return &postgresRepo{pool: pool}
}

const countByUserSQL = `
SELECT
	(SELECT COUNT(*)::int FROM reviews WHERE user_id = $1::uuid),
	(SELECT COUNT(*)::int FROM place_submissions WHERE user_id = $1::uuid),
	(SELECT COUNT(*)::int FROM issue_reports WHERE user_id = $1::uuid)
`

func (r *postgresRepo) CountByUser(ctx context.Context, userID string) (*Stats, error) {
	var stats Stats
	err := r.pool.QueryRow(ctx, countByUserSQL, userID).Scan(
		&stats.ReviewCount,
		&stats.PlaceSubmissionCount,
		&stats.IssueReportCount,
	)
	if err != nil {
		return nil, fmt.Errorf("count user stats: %w", err)
	}
	return &stats, nil
}
