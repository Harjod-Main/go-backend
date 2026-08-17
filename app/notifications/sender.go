package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
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

type webPushSendFunc func(ctx context.Context, message []byte, sub WebPushSubscriptionRequest, opts *webpush.Options) error

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

func buildWebPushPayload(evt NotificationEvent) (title, body, url string) {
	return evt.Title, evt.Body, evt.URL
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

func (s *Sender) sendWebPush(ctx context.Context, userID string, evt NotificationEvent) error {
	privateKey := os.Getenv("WEB_PUSH_VAPID_PRIVATE_KEY")
	publicKey := os.Getenv("WEB_PUSH_VAPID_PUBLIC_KEY")
	subject := os.Getenv("WEB_PUSH_VAPID_SUBJECT")
	if subject == "" {
		subject = "mailto:support@harjod.app"
	}
	if privateKey == "" || publicKey == "" {
		// Allow running dev without secrets.
		slog.Warn("web push VAPID secrets missing; skipping web push", "has_private", privateKey != "", "has_public", publicKey != "")
		return nil
	}

	titles, body, url := buildWebPushPayload(evt)

	// webpush-go expects message bytes (we send JSON so the SW can parse it)
	payload := map[string]any{
		"title": evt.Type + " " + titles,
		"body":  body,
		"url":   url,
		"type":  evt.Type,
	}
	// If title already set, use it (avoid duplication).
	if evt.Title != "" {
		payload["title"] = evt.Title
	}

	message, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal web push payload: %w", err)
	}

	subscriptions, err := s.repo.ListWebPushSubscriptions(ctx, userID)
	if err != nil {
		return fmt.Errorf("list web push subscriptions: %w", err)
	}
	if len(subscriptions) == 0 {
		return nil
	}

	opts := &webpush.Options{
		Subscriber:      subject,
		TTL:             30,
		VAPIDPublicKey:  publicKey,
		VAPIDPrivateKey: privateKey,
		HTTPClient:      s.httpClient,
	}
	send := s.webPushSend
	if send == nil {
		send = sendOneWebPush
	}

	g := new(errgroup.Group)
	var mu sync.Mutex
	var sendErr error
	stale := make([]string, 0)
	for _, sub := range subscriptions {
		sub := sub
		g.Go(func() error {
			err := s.webPushGate.Do(ctx, func() error {
				return send(ctx, message, sub, opts)
			})
			if err == nil {
				return nil
			}
			mu.Lock()
			defer mu.Unlock()
			if errors.Is(err, errStalePushDestination) {
				stale = append(stale, sub.Endpoint)
				return nil
			}
			sendErr = errors.Join(sendErr, err)
			return nil
		})
	}
	_ = g.Wait()
	s.dropStaleWebPush(ctx, userID, stale)
	return sendErr
}

func sendOneWebPush(ctx context.Context, message []byte, sub WebPushSubscriptionRequest, opts *webpush.Options) error {
	target := &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys: webpush.Keys{
			Auth:   sub.Keys.Auth,
			P256dh: sub.Keys.P256dh,
		},
	}
	resp, err := webpush.SendNotificationWithContext(ctx, message, target, opts)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		return errors.Join(errProviderOutage, fmt.Errorf("webpush send: %w", err))
	}
	defer resp.Body.Close()
	if isGoneHTTPStatus(resp.StatusCode) {
		return errStalePushDestination
	}
	return classifyHTTPStatus("webpush", resp.StatusCode)
}

func (s *Sender) sendExpoPush(ctx context.Context, userID string, evt NotificationEvent) error {
	expoAccessToken := os.Getenv("EXPO_ACCESS_TOKEN")
	if expoAccessToken == "" {
		// Allow running without iOS secrets.
		slog.Warn("EXPO_ACCESS_TOKEN missing; skipping iOS push", "user_id", userID)
		return nil
	}

	tokens, err := s.repo.ListIOSPushTokens(ctx, userID)
	if err != nil {
		return fmt.Errorf("list ios push tokens: %w", err)
	}
	if len(tokens) == 0 {
		return nil
	}

	client := s.httpClient
	if client == nil {
		client = &http.Client{Timeout: pushSendTimeout}
	}
	expoURL := s.expoPushURL
	if expoURL == "" {
		expoURL = expoPushSendURL
	}

	messages := make([]expoPushMessage, 0, len(tokens))
	for _, token := range tokens {
		to := strings.TrimSpace(token.Token)
		if to == "" {
			continue
		}
		messages = append(messages, expoPushMessage{
			To:    to,
			Title: evt.Title,
			Body:  evt.Body,
			Data: map[string]any{
				"url":  evt.URL,
				"type": evt.Type,
			},
		})
	}
	if len(messages) == 0 {
		return nil
	}

	g := new(errgroup.Group)
	var mu sync.Mutex
	var sendErr error
	stale := make([]string, 0)
	for _, batch := range chunkBy(messages, expoPushMaxBatch) {
		batch := batch
		g.Go(func() error {
			var batchStale []string
			err := s.expoGate.Do(ctx, func() error {
				var inner error
				batchStale, inner = sendExpoPushBatch(ctx, client, expoURL, expoAccessToken, batch)
				return inner
			})
			mu.Lock()
			defer mu.Unlock()
			stale = append(stale, batchStale...)
			if err != nil {
				sendErr = errors.Join(sendErr, err)
			}
			return nil
		})
	}
	_ = g.Wait()
	s.dropStaleIOSTokens(ctx, userID, stale)
	return sendErr
}

func sendExpoPushBatch(ctx context.Context, client *http.Client, expoURL, accessToken string, messages []expoPushMessage) ([]string, error) {
	if len(messages) == 0 {
		return nil, nil
	}
	raw, err := json.Marshal(messages)
	if err != nil {
		return nil, fmt.Errorf("marshal expo request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, expoURL, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("build expo request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
		return nil, errors.Join(errProviderOutage, fmt.Errorf("expo request: %w", err))
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxExpoResponseBytes+1))
	if err != nil {
		return nil, errors.Join(errProviderOutage, fmt.Errorf("read expo response: %w", err))
	}
	if len(body) > maxExpoResponseBytes {
		return nil, errors.Join(errProviderOutage, fmt.Errorf("expo response too large"))
	}
	if statusErr := classifyHTTPStatus("expo", resp.StatusCode); statusErr != nil {
		return nil, statusErr
	}

	tickets, err := parseExpoTickets(body, len(messages))
	if err != nil {
		return nil, err
	}

	var stale []string
	var sendErr error
	for i, ticket := range tickets {
		if err := classifyExpoTicket(ticket); err != nil {
			if errors.Is(err, errStalePushDestination) {
				stale = append(stale, messages[i].To)
				continue
			}
			sendErr = errors.Join(sendErr, err)
		}
	}
	return stale, sendErr
}

func (s *Sender) dropStaleWebPush(ctx context.Context, userID string, endpoints []string) {
	endpoints = uniqueStrings(endpoints)
	if len(endpoints) == 0 || s.repo == nil {
		return
	}
	if err := s.repo.DeleteWebPushSubscriptions(ctx, userID, endpoints); err != nil {
		slog.Error("delete stale web push subscriptions failed", "user_id", userID, "count", len(endpoints), "error", err)
		return
	}
	slog.Info("deleted stale web push subscriptions", "user_id", userID, "count", len(endpoints))
}

func (s *Sender) dropStaleIOSTokens(ctx context.Context, userID string, tokens []string) {
	tokens = uniqueStrings(tokens)
	if len(tokens) == 0 || s.repo == nil {
		return
	}
	if err := s.repo.DeleteIOSPushTokens(ctx, userID, tokens); err != nil {
		slog.Error("delete stale ios push tokens failed", "user_id", userID, "count", len(tokens), "error", err)
		return
	}
	slog.Info("deleted stale ios push tokens", "user_id", userID, "count", len(tokens))
}
