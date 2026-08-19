package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/RinTanth/go-backend/app/auth/supabaseauth"
	"github.com/RinTanth/go-common/app"
	"github.com/RinTanth/go-common/wrapper"
	"github.com/gin-gonic/gin"
)

type limiter interface {
	allow(key string) (bool, time.Duration)
}

// IPRateLimit limits requests per client IP using a fixed-window counter in process memory.
// Boundary burst (dump leftover quota then a full new window) is acceptable on public
// map/autocomplete reads. Writes use ActorRateLimit (token bucket) instead.
//
// Scaling note: each replica keeps its own counter, so effective quota is limit×N replicas
// (e.g. 60/min becomes 60×N behind a load balancer). Fine for single-replica deploys today.
// When scaling horizontally, replace with a shared store (Redis) or distributed token bucket.
//
// Expired entries are evicted lazily during allow() (no background goroutine) so tests and
// repeated handler construction do not leak goroutines.
func IPRateLimit(limit int, window time.Duration) gin.HandlerFunc {
	return rateLimit(newFixedWindowLimiter(limit, window), func(c *gin.Context) string {
		return c.ClientIP()
	})
}

// ActorRateLimit prefers authenticated user ID and falls back to client IP for
// anonymous traffic. Token bucket so a dump at the end of one minute cannot be
// followed by a full new dump at the start of the next.
func ActorRateLimit(limit int, window time.Duration) gin.HandlerFunc {
	return rateLimit(newTokenBucketLimiter(limit, window), func(c *gin.Context) string {
		if claims, ok := supabaseauth.ClaimsFromGin(c); ok && claims.Sub != "" {
			return "user:" + claims.Sub
		}
		return "ip:" + c.ClientIP()
	})
}

func rateLimit(limiter limiter, keyFn func(*gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := keyFn(c)
		if key == "" {
			key = c.ClientIP()
		}
		allowed, retryAfter := limiter.allow(key)
		if allowed {
			c.Next()
			return
		}

		if retryAfter > 0 {
			sec := int(retryAfter.Round(time.Second) / time.Second)
			if sec < 1 {
				sec = 1
			}
			c.Header("Retry-After", strconv.Itoa(sec))
		}
		wrapper.Respond(c, wrapper.ResponseOption[any]{
			HTTPStatus: http.StatusTooManyRequests,
			Code:       app.CodeBadRequest,
			Message:    app.Message("Too Many Requests"),
		})
		c.Abort()
	}
}

func normalizeLimitWindow(limit int, window time.Duration) (int, time.Duration) {
	if limit <= 0 {
		limit = 60
	}
	if window <= 0 {
		window = time.Minute
	}
	return limit, window
}

type fixedWindowLimiter struct {
	mu          sync.Mutex
	limit       int
	window      time.Duration
	visitors    map[string]*fixedVisitor
	lastCleanup time.Time
}

type fixedVisitor struct {
	count       int
	windowStart time.Time
}

func newFixedWindowLimiter(limit int, window time.Duration) *fixedWindowLimiter {
	limit, window = normalizeLimitWindow(limit, window)
	return &fixedWindowLimiter{
		limit:    limit,
		window:   window,
		visitors: make(map[string]*fixedVisitor),
	}
}

func (l *fixedWindowLimiter) allow(key string) (bool, time.Duration) {
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	if now.Sub(l.lastCleanup) >= l.window {
		for key, v := range l.visitors {
			if now.Sub(v.windowStart) >= l.window {
				delete(l.visitors, key)
			}
		}
		l.lastCleanup = now
	}

	v, ok := l.visitors[key]
	if !ok || now.Sub(v.windowStart) >= l.window {
		l.visitors[key] = &fixedVisitor{count: 1, windowStart: now}
		return true, 0
	}

	if v.count >= l.limit {
		retryAfter := l.window - now.Sub(v.windowStart)
		if retryAfter < time.Second {
			retryAfter = time.Second
		}
		return false, retryAfter
	}

	v.count++
	return true, 0
}

type tokenBucketLimiter struct {
	mu          sync.Mutex
	limit       float64
	rate        float64 // tokens per second
	window      time.Duration
	visitors    map[string]*tokenBucket
	lastCleanup time.Time
	now         func() time.Time
}

type tokenBucket struct {
	tokens float64
	last   time.Time
}

func newTokenBucketLimiter(limit int, window time.Duration) *tokenBucketLimiter {
	limit, window = normalizeLimitWindow(limit, window)
	return &tokenBucketLimiter{
		limit:    float64(limit),
		rate:     float64(limit) / window.Seconds(),
		window:   window,
		visitors: make(map[string]*tokenBucket),
		now:      time.Now,
	}
}

func (l *tokenBucketLimiter) allow(key string) (bool, time.Duration) {
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	if now.Sub(l.lastCleanup) >= l.window {
		for k, v := range l.visitors {
			if now.Sub(v.last) >= l.window {
				delete(l.visitors, k)
			}
		}
		l.lastCleanup = now
	}

	v, ok := l.visitors[key]
	if !ok {
		l.visitors[key] = &tokenBucket{tokens: l.limit - 1, last: now}
		return true, 0
	}

	elapsed := now.Sub(v.last).Seconds()
	if elapsed > 0 {
		v.tokens += elapsed * l.rate
		if v.tokens > l.limit {
			v.tokens = l.limit
		}
		v.last = now
	}

	if v.tokens < 1 {
		need := 1 - v.tokens
		retryAfter := time.Duration(need / l.rate * float64(time.Second))
		if retryAfter < time.Second {
			retryAfter = time.Second
		}
		return false, retryAfter
	}

	v.tokens--
	return true, 0
}
