package notifications

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

const (
	maxPushAttempts       = 5
	pushSendTimeout       = 15 * time.Second
	pushPollEvery         = 2 * time.Second
	pushStaleAfter        = 2 * time.Minute
	pushCloseWait         = 18 * time.Second
	pushPurgeEvery        = 10 * time.Minute
	pushWorkerCount       = 6
	webPushMaxConcurrent  = 4
	expoPushMaxConcurrent = 4
)

type Sender struct {
	repo        NotificationsRepository
	queue       JobQueue
	cancel      context.CancelFunc
	done        chan struct{}
	wake        chan struct{}
	workers     *semaphore.Weighted
	inFlight    sync.WaitGroup
	webPushGate *providerGate
	expoGate    *providerGate
	httpClient  *http.Client
	expoPushURL string
	webPushSend webPushSendFunc

	// deliverFn lets tests stub provider calls without hitting the network.
	deliverFn func(ctx context.Context, userID string, evt NotificationEvent) error
}

func NewSender(repo NotificationsRepository) *Sender {
	var queue JobQueue
	if q, ok := repo.(JobQueue); ok {
		queue = q
	}
	return newSender(repo, queue, true)
}

func newSender(repo NotificationsRepository, queue JobQueue, startWorker bool) *Sender {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Sender{
		repo:        repo,
		queue:       queue,
		cancel:      cancel,
		done:        make(chan struct{}),
		wake:        make(chan struct{}, 1),
		workers:     semaphore.NewWeighted(pushWorkerCount),
		webPushGate: newProviderGate("webpush", webPushMaxConcurrent),
		expoGate:    newProviderGate("expo", expoPushMaxConcurrent),
		httpClient:  &http.Client{Timeout: pushSendTimeout},
		expoPushURL: expoPushSendURL,
		webPushSend: sendOneWebPush,
	}
	if !startWorker {
		close(s.done)
		return s
	}
	go s.run(ctx)
	return s
}

func (s *Sender) Close() {
	if s == nil {
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	if s.done == nil {
		return
	}
	select {
	case <-s.done:
	case <-time.After(pushCloseWait):
		slog.Warn("notification worker did not stop before timeout")
	}
}

// SendToUser enqueues a push job and returns. Web Push / Expo delivery happens
// on a background worker with retries, so review/check-in/submission handlers
// are not blocked on provider latency.
func (s *Sender) SendToUser(ctx context.Context, userID string, evt NotificationEvent) error {
	if s == nil {
		return nil
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil
	}
	if s.queue == nil {
		slog.Error("notification queue missing; dropping push", "user_id", userID, "type", evt.Type)
		return nil
	}
	if err := s.queue.Enqueue(ctx, userID, evt); err != nil {
		slog.Error("enqueue notification failed", "user_id", userID, "type", evt.Type, "error", err)
		return err
	}
	s.nudge()
	return nil
}

func (s *Sender) nudge() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Sender) run(ctx context.Context) {
	defer close(s.done)
	ticker := time.NewTicker(pushPollEvery)
	defer ticker.Stop()
	maintain := time.NewTicker(pushPurgeEvery)
	defer maintain.Stop()

	s.maintain(ctx)

	for {
		select {
		case <-ctx.Done():
			s.inFlight.Wait()
			return
		case <-s.wake:
			s.drain(ctx)
		case <-ticker.C:
			s.drain(ctx)
		case <-maintain.C:
			s.maintain(ctx)
		}
	}
}

func (s *Sender) maintain(ctx context.Context) {
	if s.queue == nil {
		return
	}

	var purge JobPurgeResult
	result, err := s.queue.PurgeExpired(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		slog.Error("purge notification jobs failed", "error", err)
	} else {
		purge = result
	}

	stats, err := s.queue.Stats(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		slog.Error("notification job stats failed", "error", err)
		return
	}

	slog.Info("notification job metrics",
		"pending", stats.Pending,
		"processing", stats.Processing,
		"done", stats.Done,
		"failed", stats.Failed,
		"retried", stats.Retried,
		"retry_attempts", stats.RetryAttempts,
		"deleted_done", purge.DeletedDone,
		"deleted_failed", purge.DeletedFailed,
		"web_push_circuit", s.webPushGate.breaker.State(),
		"expo_circuit", s.expoGate.breaker.State(),
	)
}

func (s *Sender) drain(ctx context.Context) {
	if s.queue == nil {
		return
	}
	if err := s.queue.ReclaimStale(ctx, pushStaleAfter); err != nil {
		if ctx.Err() != nil {
			return
		}
		slog.Error("reclaim stale notification jobs failed", "error", err)
	}

	for {
		if ctx.Err() != nil {
			return
		}
		if !s.workers.TryAcquire(1) {
			return
		}
		job, err := s.queue.Claim(ctx)
		if err != nil {
			s.workers.Release(1)
			if ctx.Err() != nil {
				return
			}
			slog.Error("claim notification job failed", "error", err)
			return
		}
		if job == nil {
			s.workers.Release(1)
			return
		}
		s.inFlight.Add(1)
		go func(job *NotificationJob) {
			defer s.inFlight.Done()
			defer s.workers.Release(1)
			s.processJob(job)
			s.nudge()
		}(job)
	}
}

func (s *Sender) processJob(job *NotificationJob) {
	if job == nil {
		return
	}
	sendCtx, cancel := context.WithTimeout(context.Background(), pushSendTimeout)
	defer cancel()

	err := s.deliver(sendCtx, job.UserID, job.Event)
	if err == nil {
		if completeErr := s.queue.Complete(context.Background(), job.JobID); completeErr != nil {
			slog.Error("complete notification job failed", "job_id", job.JobID, "error", completeErr)
		}
		return
	}

	attempts := job.Attempts + 1
	maxAttempts := job.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = maxPushAttempts
	}
	if attempts >= maxAttempts {
		slog.Error("notification job exhausted retries",
			"job_id", job.JobID,
			"user_id", job.UserID,
			"type", job.Event.Type,
			"attempts", attempts,
			"error", err,
		)
		if failErr := s.queue.Fail(context.Background(), job.JobID, err.Error()); failErr != nil {
			slog.Error("fail notification job failed", "job_id", job.JobID, "error", failErr)
		}
		return
	}

	delay := retryDelay(attempts)
	if errors.Is(err, errCircuitOpen) && delay < breakerOpenFor {
		delay = breakerOpenFor
	}
	next := time.Now().UTC().Add(delay)
	slog.Warn("notification job scheduled for retry",
		"job_id", job.JobID,
		"user_id", job.UserID,
		"type", job.Event.Type,
		"attempts", attempts,
		"next_attempt_at", next,
		"error", err,
	)
	if retryErr := s.queue.Retry(context.Background(), job.JobID, attempts, next, err.Error()); retryErr != nil {
		slog.Error("retry notification job failed", "job_id", job.JobID, "error", retryErr)
	}
}

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := time.Duration(1<<attempt) * time.Second
	const capDelay = 5 * time.Minute
	if delay > capDelay {
		return capDelay
	}
	return delay
}

func (s *Sender) deliver(ctx context.Context, userID string, evt NotificationEvent) error {
	if s.deliverFn != nil {
		return s.deliverFn(ctx, userID, evt)
	}

	prefs, err := s.repo.GetPreferences(ctx, userID)
	if err != nil {
		return fmt.Errorf("get preferences: %w", err)
	}
	if prefs != nil && !prefs.NotificationsEnabled {
		return nil
	}

	var webErr, expoErr error
	g := new(errgroup.Group)
	g.Go(func() error {
		webErr = s.sendWebPush(ctx, userID, evt)
		return nil
	})
	g.Go(func() error {
		expoErr = s.sendExpoPush(ctx, userID, evt)
		return nil
	})
	_ = g.Wait()
	return errors.Join(webErr, expoErr)
}
