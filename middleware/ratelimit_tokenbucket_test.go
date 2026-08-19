package middleware

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTokenBucket_RejectsFixedWindowBoundaryBurst(t *testing.T) {
	r := require.New(t)
	now := time.Date(2026, 1, 1, 12, 0, 59, 0, time.UTC)
	l := newTokenBucketLimiter(2, 10*time.Second)
	l.now = func() time.Time { return now }

	ok, _ := l.allow("user-1")
	r.True(ok)
	ok, _ = l.allow("user-1")
	r.True(ok)
	ok, _ = l.allow("user-1")
	r.False(ok)

	// Almost a full window later: a fixed window would reset and allow 2 more.
	// Token bucket has only refilled ~1.8 tokens, so at most one extra request.
	now = now.Add(9 * time.Second)
	ok, _ = l.allow("user-1")
	r.True(ok)
	ok, retryAfter := l.allow("user-1")
	r.False(ok)
	r.GreaterOrEqual(retryAfter, time.Second)
}

func TestTokenBucket_RefillsFullQuotaAfterIdleWindow(t *testing.T) {
	r := require.New(t)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	l := newTokenBucketLimiter(2, 10*time.Second)
	l.now = func() time.Time { return now }

	ok, _ := l.allow("user-1")
	r.True(ok)
	ok, _ = l.allow("user-1")
	r.True(ok)

	now = now.Add(10 * time.Second)
	ok, _ = l.allow("user-1")
	r.True(ok)
	ok, _ = l.allow("user-1")
	r.True(ok)
	ok, _ = l.allow("user-1")
	r.False(ok)
}
