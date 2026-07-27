package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/RinTanth/go-common/app"
	"github.com/RinTanth/go-common/wrapper"
	"github.com/gin-gonic/gin"
)

// IPRateLimit limits requests per client IP using a fixed window counter.
// Exceeding the limit returns HTTP 429.
func IPRateLimit(limit int, window time.Duration) gin.HandlerFunc {
	if limit <= 0 {
		limit = 60
	}
	if window <= 0 {
		window = time.Minute
	}

	limiter := newIPLimiter(limit, window)
	return func(c *gin.Context) {
		key := c.ClientIP()
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

type ipLimiter struct {
	mu       sync.Mutex
	limit    int
	window   time.Duration
	visitors map[string]*visitor
}

type visitor struct {
	count       int
	windowStart time.Time
}

func newIPLimiter(limit int, window time.Duration) *ipLimiter {
	l := &ipLimiter{
		limit:    limit,
		window:   window,
		visitors: make(map[string]*visitor),
	}
	go l.cleanupLoop()
	return l
}

func (l *ipLimiter) allow(key string) (bool, time.Duration) {
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	v, ok := l.visitors[key]
	if !ok || now.Sub(v.windowStart) >= l.window {
		l.visitors[key] = &visitor{count: 1, windowStart: now}
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

func (l *ipLimiter) cleanupLoop() {
	ticker := time.NewTicker(l.window)
	defer ticker.Stop()
	for range ticker.C {
		l.cleanup()
	}
}

func (l *ipLimiter) cleanup() {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	for key, v := range l.visitors {
		if now.Sub(v.windowStart) >= l.window {
			delete(l.visitors, key)
		}
	}
}
