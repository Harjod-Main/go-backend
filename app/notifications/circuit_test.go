package notifications

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCircuitBreaker_OpensAfterConsecutiveOutages(t *testing.T) {
	r := require.New(t)
	now := time.Now()
	c := newCircuitBreaker("expo")
	c.now = func() time.Time { return now }

	for i := 0; i < breakerFailureLimit; i++ {
		r.NoError(c.Allow())
		c.Record(errProviderOutage)
	}
	r.ErrorIs(c.Allow(), errCircuitOpen)
	r.Equal("open", c.State())

	now = now.Add(breakerOpenFor + time.Millisecond)
	r.NoError(c.Allow())
	r.ErrorIs(c.Allow(), errCircuitOpen)
	c.Record(nil)
	r.NoError(c.Allow())
	r.Equal("closed", c.State())
}

func TestCircuitBreaker_HalfOpenFailureReopens(t *testing.T) {
	r := require.New(t)
	now := time.Now()
	c := newCircuitBreaker("webpush")
	c.now = func() time.Time { return now }

	for i := 0; i < breakerFailureLimit; i++ {
		r.NoError(c.Allow())
		c.Record(errProviderOutage)
	}
	now = now.Add(breakerOpenFor + time.Millisecond)
	r.NoError(c.Allow())
	c.Record(errProviderOutage)
	r.ErrorIs(c.Allow(), errCircuitOpen)
}

func TestCircuitBreaker_ClientErrorsDoNotOpen(t *testing.T) {
	r := require.New(t)
	c := newCircuitBreaker("expo")
	for i := 0; i < breakerFailureLimit+3; i++ {
		r.NoError(c.Allow())
		c.Record(errors.New("expo non-2xx status: 400"))
	}
	r.NoError(c.Allow())
	r.Equal("closed", c.State())
}

func TestClassifyHTTPStatus(t *testing.T) {
	r := require.New(t)
	r.NoError(classifyHTTPStatus("expo", 200))
	r.ErrorIs(classifyHTTPStatus("expo", 503), errProviderOutage)
	r.ErrorIs(classifyHTTPStatus("expo", 429), errProviderOutage)
	err := classifyHTTPStatus("expo", 400)
	r.Error(err)
	r.False(errors.Is(err, errProviderOutage))
	r.False(errors.Is(classifyHTTPStatus("webpush", 410), errStalePushDestination))
	r.True(isGoneHTTPStatus(404))
	r.True(isGoneHTTPStatus(410))
	r.False(isGoneHTTPStatus(403))
}

func TestProviderGate_BoundsConcurrency(t *testing.T) {
	r := require.New(t)
	g := newProviderGate("expo", 2)
	var current, max atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = g.Do(context.Background(), func() error {
				n := current.Add(1)
				for {
					old := max.Load()
					if n <= old || max.CompareAndSwap(old, n) {
						break
					}
				}
				time.Sleep(25 * time.Millisecond)
				current.Add(-1)
				return nil
			})
		}()
	}
	wg.Wait()
	r.LessOrEqual(max.Load(), int32(2))
	r.GreaterOrEqual(max.Load(), int32(2))
}

func TestProviderGate_RejectsWhenOpen(t *testing.T) {
	r := require.New(t)
	g := newProviderGate("webpush", 4)
	for i := 0; i < breakerFailureLimit; i++ {
		r.ErrorIs(g.Do(context.Background(), func() error { return errProviderOutage }), errProviderOutage)
	}
	started := false
	err := g.Do(context.Background(), func() error {
		started = true
		return nil
	})
	r.ErrorIs(err, errCircuitOpen)
	r.False(started)
}
