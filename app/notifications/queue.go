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
}
