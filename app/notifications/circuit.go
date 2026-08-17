package notifications

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"
)

var (
	errCircuitOpen    = errors.New("push provider circuit open")
	errProviderOutage = errors.New("push provider outage")
)

const (
	breakerFailureLimit = 5
	breakerOpenFor      = 30 * time.Second
)

type circuitState int

const (
	circuitClosed circuitState = iota
	circuitOpen
	circuitHalfOpen
)

func (s circuitState) String() string {
	switch s {
	case circuitOpen:
		return "open"
	case circuitHalfOpen:
		return "half_open"
	default:
		return "closed"
	}
}

type circuitBreaker struct {
	name         string
	failureLimit int
	openFor      time.Duration
	now          func() time.Time

	mu               sync.Mutex
	state            circuitState
	consecutive      int
	openedAt         time.Time
	halfOpenInFlight bool
}

func newCircuitBreaker(name string) *circuitBreaker {
	return &circuitBreaker{
		name:         name,
		failureLimit: breakerFailureLimit,
		openFor:      breakerOpenFor,
		now:          time.Now,
	}
}

func (c *circuitBreaker) State() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.advanceLocked(c.now())
	return c.state.String()
}

func (c *circuitBreaker) Allow() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	c.advanceLocked(now)
	switch c.state {
	case circuitOpen:
		return errCircuitOpen
	case circuitHalfOpen:
		if c.halfOpenInFlight {
			return errCircuitOpen
		}
		c.halfOpenInFlight = true
		return nil
	default:
		return nil
	}
}

func (c *circuitBreaker) CancelProbe() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state == circuitHalfOpen {
		c.halfOpenInFlight = false
	}
}

func (c *circuitBreaker) Record(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state == circuitHalfOpen {
		c.halfOpenInFlight = false
	}
	if !isBreakerFailure(err) {
		if err == nil && (c.state != circuitClosed || c.consecutive != 0) {
			if c.state != circuitClosed {
				slog.Info("push provider circuit closed", "provider", c.name)
			}
			c.state = circuitClosed
			c.consecutive = 0
		}
		return
	}
	c.consecutive++
	if c.state == circuitHalfOpen || c.consecutive >= c.failureLimit {
		c.state = circuitOpen
		c.openedAt = c.now()
		slog.Warn("push provider circuit opened",
			"provider", c.name,
			"failures", c.consecutive,
			"open_for", c.openFor,
		)
	}
}

func (c *circuitBreaker) advanceLocked(now time.Time) {
	if c.state == circuitOpen && now.Sub(c.openedAt) >= c.openFor {
		c.state = circuitHalfOpen
		c.halfOpenInFlight = false
	}
}

func isBreakerFailure(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errCircuitOpen) || errors.Is(err, context.Canceled) {
		return false
	}
	return errors.Is(err, errProviderOutage) || errors.Is(err, context.DeadlineExceeded)
}

func classifyHTTPStatus(provider string, status int) error {
	if status >= 200 && status < 300 {
		return nil
	}
	err := fmt.Errorf("%s non-2xx status: %d", provider, status)
	if status == 408 || status == 429 || status >= 500 {
		return errors.Join(errProviderOutage, err)
	}
	return err
}

type providerGate struct {
	breaker *circuitBreaker
	sem     *semaphore.Weighted
}

func newProviderGate(name string, maxConcurrent int64) *providerGate {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	return &providerGate{
		breaker: newCircuitBreaker(name),
		sem:     semaphore.NewWeighted(maxConcurrent),
	}
}

func (g *providerGate) Do(ctx context.Context, fn func() error) error {
	if err := g.breaker.Allow(); err != nil {
		return err
	}
	if err := g.sem.Acquire(ctx, 1); err != nil {
		g.breaker.CancelProbe()
		return err
	}
	defer g.sem.Release(1)
	err := fn()
	g.breaker.Record(err)
	return err
}
