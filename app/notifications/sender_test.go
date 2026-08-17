package notifications

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type stubQueue struct {
	mu         sync.Mutex
	enqueued   []NotificationJob
	enqueueErr error
	complete   []string
	retried    []NotificationJob
	failed     []string
}

func (q *stubQueue) Enqueue(_ context.Context, userID string, evt NotificationEvent) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.enqueueErr != nil {
		return q.enqueueErr
	}
	q.enqueued = append(q.enqueued, NotificationJob{
		JobID:       "job-1",
		UserID:      userID,
		Event:       evt,
		Attempts:    0,
		MaxAttempts: maxPushAttempts,
	})
	return nil
}

func (q *stubQueue) Claim(context.Context) (*NotificationJob, error) { return nil, nil }
func (q *stubQueue) Complete(_ context.Context, jobID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.complete = append(q.complete, jobID)
	return nil
}
func (q *stubQueue) Retry(_ context.Context, jobID string, attempts int, _ time.Time, _ string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.retried = append(q.retried, NotificationJob{JobID: jobID, Attempts: attempts})
	return nil
}
func (q *stubQueue) Fail(_ context.Context, jobID string, _ string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.failed = append(q.failed, jobID)
	return nil
}
func (q *stubQueue) ReclaimStale(context.Context, time.Duration) error { return nil }

func TestSendToUser_EnqueuesWithoutCallingProviders(t *testing.T) {
	r := require.New(t)
	q := &stubQueue{}
	s := newSender(nil, q, false)
	s.deliverFn = func(context.Context, string, NotificationEvent) error {
		t.Fatal("deliver must not run on the request path")
		return nil
	}

	start := time.Now()
	err := s.SendToUser(context.Background(), "11111111-1111-1111-1111-111111111111", NotificationEvent{
		Type:  "review",
		Title: "Review submitted",
	})
	r.NoError(err)
	r.Less(time.Since(start), 100*time.Millisecond)
	r.Len(q.enqueued, 1)
	r.Equal("review", q.enqueued[0].Event.Type)
}

func TestSendToUser_DoesNotWaitWhenEnqueueFails(t *testing.T) {
	r := require.New(t)
	q := &stubQueue{enqueueErr: errors.New("db down")}
	s := newSender(nil, q, false)

	err := s.SendToUser(context.Background(), "11111111-1111-1111-1111-111111111111", NotificationEvent{Type: "checkin"})
	r.EqualError(err, "db down")
}

func TestProcessJob_CompletesOnSuccess(t *testing.T) {
	r := require.New(t)
	q := &stubQueue{}
	s := newSender(nil, q, false)
	s.deliverFn = func(context.Context, string, NotificationEvent) error { return nil }

	s.processJob(&NotificationJob{
		JobID:       "job-ok",
		UserID:      "u1",
		Event:       NotificationEvent{Type: "submission"},
		MaxAttempts: 5,
	})
	r.Equal([]string{"job-ok"}, q.complete)
	r.Empty(q.retried)
	r.Empty(q.failed)
}

func TestProcessJob_RetriesThenFails(t *testing.T) {
	r := require.New(t)
	q := &stubQueue{}
	s := newSender(nil, q, false)
	s.deliverFn = func(context.Context, string, NotificationEvent) error {
		return errors.New("expo timeout")
	}

	s.processJob(&NotificationJob{
		JobID:       "job-retry",
		UserID:      "u1",
		Event:       NotificationEvent{Type: "review"},
		Attempts:    0,
		MaxAttempts: 3,
	})
	r.Empty(q.complete)
	r.Len(q.retried, 1)
	r.Equal(1, q.retried[0].Attempts)

	s.processJob(&NotificationJob{
		JobID:       "job-retry",
		UserID:      "u1",
		Event:       NotificationEvent{Type: "review"},
		Attempts:    2,
		MaxAttempts: 3,
	})
	r.Equal([]string{"job-retry"}, q.failed)
}

func TestRetryDelay_GrowsAndCaps(t *testing.T) {
	r := require.New(t)
	r.Equal(2*time.Second, retryDelay(1))
	r.Equal(4*time.Second, retryDelay(2))
	r.Equal(5*time.Minute, retryDelay(12))
}
