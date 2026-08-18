package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"

	webpush "github.com/SherClockHolmes/webpush-go"
	"golang.org/x/sync/errgroup"
)

type webPushSendFunc func(ctx context.Context, message []byte, sub WebPushSubscriptionRequest, opts *webpush.Options) error

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

	payload := map[string]any{
		"title": evt.Title,
		"body":  evt.Body,
		"url":   evt.URL,
		"type":  evt.Type,
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
