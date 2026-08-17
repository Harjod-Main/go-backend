package notifications

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	expoPushSendURL      = "https://exp.host/--/api/v2/push/send"
	expoPushMaxBatch     = 100
	maxExpoResponseBytes = 1 << 20
)

type expoPushMessage struct {
	To    string         `json:"to"`
	Title string         `json:"title,omitempty"`
	Body  string         `json:"body,omitempty"`
	Data  map[string]any `json:"data,omitempty"`
}

type expoPushTicket struct {
	Status  string `json:"status"`
	ID      string `json:"id"`
	Message string `json:"message"`
	Details struct {
		Error string `json:"error"`
	} `json:"details"`
}

type expoPushResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

func chunkBy[T any](items []T, size int) [][]T {
	if size < 1 {
		size = 1
	}
	if len(items) == 0 {
		return nil
	}
	out := make([][]T, 0, (len(items)+size-1)/size)
	for i := 0; i < len(items); i += size {
		end := i + size
		if end > len(items) {
			end = len(items)
		}
		out = append(out, items[i:end])
	}
	return out
}

func uniqueStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func parseExpoTickets(raw []byte, expect int) ([]expoPushTicket, error) {
	var resp expoPushResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode expo response: %w", err)
	}
	if len(resp.Errors) > 0 {
		msg := strings.TrimSpace(resp.Errors[0].Message)
		if msg == "" {
			msg = resp.Errors[0].Code
		}
		return nil, fmt.Errorf("expo api error: %s", msg)
	}
	if len(resp.Data) == 0 || string(resp.Data) == "null" {
		return nil, fmt.Errorf("expo response missing data")
	}

	var tickets []expoPushTicket
	if resp.Data[0] == '[' {
		if err := json.Unmarshal(resp.Data, &tickets); err != nil {
			return nil, fmt.Errorf("decode expo tickets: %w", err)
		}
	} else {
		var one expoPushTicket
		if err := json.Unmarshal(resp.Data, &one); err != nil {
			return nil, fmt.Errorf("decode expo ticket: %w", err)
		}
		tickets = []expoPushTicket{one}
	}
	if len(tickets) != expect {
		return nil, errors.Join(errProviderOutage, fmt.Errorf("expo ticket count %d != %d", len(tickets), expect))
	}
	return tickets, nil
}

func classifyExpoTicket(ticket expoPushTicket) error {
	if ticket.Status == "" || ticket.Status == "ok" {
		return nil
	}
	if isStaleExpoError(ticket.Details.Error) {
		return errStalePushDestination
	}
	msg := strings.TrimSpace(ticket.Message)
	if msg == "" {
		msg = ticket.Details.Error
	}
	if msg == "" {
		msg = ticket.Status
	}
	err := fmt.Errorf("expo ticket error: %s", msg)
	if ticket.Details.Error == "MessageRateExceeded" {
		return errors.Join(errProviderOutage, err)
	}
	return err
}

func isStaleExpoError(code string) bool {
	switch strings.TrimSpace(code) {
	case "DeviceNotRegistered", "InvalidPushToken":
		return true
	default:
		return false
	}
}
