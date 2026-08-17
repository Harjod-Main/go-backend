package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

const (
	maxPushAttempts = 5
	pushSendTimeout = 15 * time.Second
	pushPollEvery   = 2 * time.Second
	pushStaleAfter  = 2 * time.Minute
	pushCloseWait   = 3 * time.Second
)

type Sender struct {
	repo   NotificationsRepository
	queue  JobQueue
	cancel context.CancelFunc
	done   chan struct{}
	wake   chan struct{}

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
		repo:   repo,
		queue:  queue,
		cancel: cancel,
		done:   make(chan struct{}),
		wake:   make(chan struct{}, 1),
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

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.wake:
			s.drain(ctx)
		case <-ticker.C:
			s.drain(ctx)
		}
	}
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
		job, err := s.queue.Claim(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("claim notification job failed", "error", err)
			return
		}
		if job == nil {
			return
		}
		s.processJob(job)
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

	next := time.Now().UTC().Add(retryDelay(attempts))
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

	webErr := s.sendWebPush(ctx, userID, evt)
	expoErr := s.sendExpoPush(ctx, userID, evt)
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
	}

	var sendErr error
	for _, sub := range subscriptions {
		s := &webpush.Subscription{
			Endpoint: sub.Endpoint,
			Keys: webpush.Keys{
				Auth:   sub.Keys.Auth,
				P256dh: sub.Keys.P256dh,
			},
		}
		resp, err := webpush.SendNotificationWithContext(ctx, message, s, opts)
		if err != nil {
			sendErr = errors.Join(sendErr, fmt.Errorf("webpush send: %w", err))
			continue
		}
		_ = resp.Body.Close()
	}
	return sendErr
}

func (s *Sender) sendExpoPush(ctx context.Context, userID string, evt NotificationEvent) error {
	expoAccessToken := os.Getenv("EXPO_ACCESS_TOKEN")
	if expoAccessToken == "" {
		// Allow running without iOS secrets.
		slog.Warn("EXPO_ACCESS_TOKEN missing; skipping iOS push", "user_id", userID)
		return nil
	}

	title := evt.Title
	body := evt.Body

	tokens, err := s.repo.ListIOSPushTokens(ctx, userID)
	if err != nil {
		return fmt.Errorf("list ios push tokens: %w", err)
	}
	if len(tokens) == 0 {
		return nil
	}

	client := &http.Client{Timeout: pushSendTimeout}

	type expoRequest struct {
		To    string `json:"to"`
		Title string `json:"title,omitempty"`
		Body  string `json:"body,omitempty"`
		Data  any    `json:"data,omitempty"`
	}

	var sendErr error
	for _, token := range tokens {
		reqBody := expoRequest{
			To:    token.Token,
			Title: title,
			Body:  body,
			Data: map[string]any{
				"url":  evt.URL,
				"type": evt.Type,
			},
		}

		raw, err := json.Marshal(reqBody)
		if err != nil {
			sendErr = errors.Join(sendErr, fmt.Errorf("marshal expo request: %w", err))
			continue
		}

		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"https://exp.host/--/api/v2/push/send",
			bytes.NewReader(raw),
		)
		if err != nil {
			sendErr = errors.Join(sendErr, fmt.Errorf("build expo request: %w", err))
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+expoAccessToken)

		resp, err := client.Do(req)
		if err != nil {
			sendErr = errors.Join(sendErr, fmt.Errorf("expo request: %w", err))
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			sendErr = errors.Join(sendErr, fmt.Errorf("expo non-2xx status: %d", resp.StatusCode))
		}
	}

	return sendErr
}
