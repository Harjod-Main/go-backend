package notifications

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var _ JobQueue = (*stubQueue)(nil)

type stubQueue struct {
	mu         sync.Mutex
	enqueued   []NotificationJob
	enqueueErr error
	complete   []string
	retried    []NotificationJob
	retryAt    []time.Time
	failed     []string
	claimJobs  []NotificationJob
	claimIdx   int
	purgeCalls int
	statsCalls int
	purge      JobPurgeResult
	stats      JobQueueStats
	purgeErr   error
	statsErr   error
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

func (q *stubQueue) Claim(context.Context) (*NotificationJob, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.claimIdx >= len(q.claimJobs) {
		return nil, nil
	}
	job := q.claimJobs[q.claimIdx]
	q.claimIdx++
	return &job, nil
}
func (q *stubQueue) Complete(_ context.Context, jobID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.complete = append(q.complete, jobID)
	return nil
}
func (q *stubQueue) Retry(_ context.Context, jobID string, attempts int, nextAttemptAt time.Time, _ string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.retried = append(q.retried, NotificationJob{JobID: jobID, Attempts: attempts})
	q.retryAt = append(q.retryAt, nextAttemptAt)
	return nil
}
func (q *stubQueue) Fail(_ context.Context, jobID string, _ string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.failed = append(q.failed, jobID)
	return nil
}
func (q *stubQueue) ReclaimStale(context.Context, time.Duration) error { return nil }

func (q *stubQueue) PurgeExpired(context.Context) (JobPurgeResult, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.purgeCalls++
	return q.purge, q.purgeErr
}

func (q *stubQueue) Stats(context.Context) (JobQueueStats, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.statsCalls++
	return q.stats, q.statsErr
}

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

func TestMaintain_PurgesAndLogsQueueStats(t *testing.T) {
	r := require.New(t)
	q := &stubQueue{
		purge: JobPurgeResult{DeletedDone: 3, DeletedFailed: 1},
		stats: JobQueueStats{Pending: 2, Failed: 4, Retried: 1, RetryAttempts: 5},
	}
	s := newSender(nil, q, false)

	s.maintain(context.Background())
	r.Equal(1, q.purgeCalls)
	r.Equal(1, q.statsCalls)
}

func TestMaintain_StillReadsStatsWhenPurgeFails(t *testing.T) {
	r := require.New(t)
	q := &stubQueue{
		purgeErr: errors.New("purge down"),
		stats:    JobQueueStats{Pending: 8},
	}
	s := newSender(nil, q, false)

	s.maintain(context.Background())
	r.Equal(1, q.purgeCalls)
	r.Equal(1, q.statsCalls)
}

func TestDrain_ProcessesJobsConcurrently(t *testing.T) {
	r := require.New(t)
	q := &stubQueue{
		claimJobs: []NotificationJob{
			{JobID: "j1", UserID: "u1", MaxAttempts: 5},
			{JobID: "j2", UserID: "u2", MaxAttempts: 5},
			{JobID: "j3", UserID: "u3", MaxAttempts: 5},
			{JobID: "j4", UserID: "u4", MaxAttempts: 5},
		},
	}
	s := newSender(nil, q, false)
	var current, max atomic.Int32
	s.deliverFn = func(context.Context, string, NotificationEvent) error {
		n := current.Add(1)
		for {
			old := max.Load()
			if n <= old || max.CompareAndSwap(old, n) {
				break
			}
		}
		time.Sleep(60 * time.Millisecond)
		current.Add(-1)
		return nil
	}

	s.drain(context.Background())
	s.inFlight.Wait()

	r.GreaterOrEqual(max.Load(), int32(2))
	r.Len(q.complete, 4)
}

func TestProcessJob_CircuitOpenUsesCooldownRetry(t *testing.T) {
	r := require.New(t)
	q := &stubQueue{}
	s := newSender(nil, q, false)
	s.deliverFn = func(context.Context, string, NotificationEvent) error {
		return errCircuitOpen
	}

	before := time.Now().UTC()
	s.processJob(&NotificationJob{
		JobID:       "job-open",
		UserID:      "u1",
		Event:       NotificationEvent{Type: "review"},
		MaxAttempts: 5,
	})
	r.Len(q.retried, 1)
	r.GreaterOrEqual(q.retryAt[0].Sub(before), breakerOpenFor-time.Second)
}
