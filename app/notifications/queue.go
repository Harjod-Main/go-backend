package notifications

import (
	"context"
	"time"
)

var _ JobQueue = (*postgresRepo)(nil)

// JobQueue is a durable outbox for push delivery. SendToUser only inserts;
// the worker claims rows with SKIP LOCKED so HTTP handlers never wait on providers.
type JobQueue interface {
	Enqueue(ctx context.Context, userID string, evt NotificationEvent) error
	Claim(ctx context.Context) (*NotificationJob, error)
	Complete(ctx context.Context, jobID string) error
	Retry(ctx context.Context, jobID string, attempts int, nextAttemptAt time.Time, lastError string) error
	Fail(ctx context.Context, jobID string, lastError string) error
	ReclaimStale(ctx context.Context, staleAfter time.Duration) error
	PurgeExpired(ctx context.Context) (JobPurgeResult, error)
	Stats(ctx context.Context) (JobQueueStats, error)
}

// JobPurgeResult is how many terminal rows a cleanup pass removed.
type JobPurgeResult struct {
	DeletedDone   int64 `json:"deleted_done"`
	DeletedFailed int64 `json:"deleted_failed"`
}

// JobQueueStats is a snapshot of outbox depth for logs and ops checks.
type JobQueueStats struct {
	Pending         int64   `json:"pending"`
	Processing      int64   `json:"processing"`
	Done            int64   `json:"done"`
	Failed          int64   `json:"failed"`
	Retried         int64   `json:"retried"`
	RetryAttempts   int64   `json:"retry_attempts"`
	OldestPendingAt *string `json:"oldest_pending_at"`
}
