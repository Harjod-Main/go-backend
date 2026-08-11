package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

type Sender struct {
	repo NotificationsRepository
}

func NewSender(repo NotificationsRepository) *Sender {
	return &Sender{repo: repo}
}

func buildWebPushPayload(evt NotificationEvent) (title, body, url string) {
	return evt.Title, evt.Body, evt.URL
}

func (s *Sender) SendToUser(ctx context.Context, userID string, evt NotificationEvent) error {
	prefs, err := s.repo.GetPreferences(ctx, userID)
	if err != nil {
		return fmt.Errorf("get preferences: %w", err)
	}
	if prefs != nil && !prefs.NotificationsEnabled {
		return nil
	}

	if err := s.sendWebPush(ctx, userID, evt); err != nil {
		// Best-effort: still try iOS.
		slog.Error("send web push failed", "user_id", userID, "error", err)
	}
	if err := s.sendExpoPush(ctx, userID, evt); err != nil {
		slog.Error("send expo push failed", "user_id", userID, "error", err)
	}

	return nil
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
			return fmt.Errorf("webpush send: %w", err)
		}
		_ = resp.Body.Close()
	}
	return nil
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

	client := &http.Client{Timeout: 15 * time.Second}

	type expoRequest struct {
		To    string `json:"to"`
		Title string `json:"title,omitempty"`
		Body  string `json:"body,omitempty"`
		Data  any    `json:"data,omitempty"`
	}

	for _, token := range tokens {
		reqBody := expoRequest{
			To:    token.Token,
			Title: title,
			Body:  body,
			Data: map[string]any{
				"url": evt.URL,
				"type": evt.Type,
			},
		}

		raw, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("marshal expo request: %w", err)
		}

		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"https://exp.host/--/api/v2/push/send",
			bytes.NewReader(raw),
		)
		if err != nil {
			return fmt.Errorf("build expo request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+expoAccessToken)

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("expo request: %w", err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("expo non-2xx status: %d", resp.StatusCode)
		}
	}

	return nil
}

