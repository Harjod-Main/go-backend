package pagination

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidCursor = errors.New("invalid cursor")
)

// Cursor is a keyset pagination position ordered by (created_at DESC, id DESC).
type Cursor struct {
	CreatedAt time.Time
	ID        string
}

// Encode returns an opaque URL-safe cursor token.
func Encode(createdAt time.Time, id string) string {
	raw := createdAt.UTC().Format(time.RFC3339Nano) + "|" + id
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// Decode parses an opaque cursor token.
func Decode(raw string) (Cursor, error) {
	if strings.TrimSpace(raw) == "" {
		return Cursor{}, ErrInvalidCursor
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return Cursor{}, ErrInvalidCursor
	}
	parts := strings.SplitN(string(decoded), "|", 2)
	if len(parts) != 2 {
		return Cursor{}, ErrInvalidCursor
	}
	createdAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return Cursor{}, ErrInvalidCursor
	}
	id := strings.TrimSpace(parts[1])
	if _, err := uuid.Parse(id); err != nil {
		return Cursor{}, ErrInvalidCursor
	}
	return Cursor{CreatedAt: createdAt.UTC(), ID: id}, nil
}

// ParseLimit reads a limit query value with defaults and caps.
func ParseLimit(raw string, defaultLimit, maxLimit int) int {
	limit := defaultLimit
	if raw == "" {
		return limit
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 || n > maxLimit {
		return limit
	}
	return n
}

// NextFromLast builds the next-page cursor from the last item on the current page.
func NextFromLast(hasMore bool, createdAt time.Time, id string) *string {
	if !hasMore {
		return nil
	}
	token := Encode(createdAt, id)
	return &token
}

// FormatDebug is useful in error messages/tests.
func FormatDebug(c Cursor) string {
	return fmt.Sprintf("%s|%s", c.CreatedAt.UTC().Format(time.RFC3339Nano), c.ID)
}
