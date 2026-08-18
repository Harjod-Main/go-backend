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

	"golang.org/x/sync/errgroup"
)

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
