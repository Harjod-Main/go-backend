package places

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestListMapCache_SingleflightCoalescesConcurrentLoads(t *testing.T) {
	r := require.New(t)
	cache := newListMapCache(time.Minute)

	var loads atomic.Int32
	start := make(chan struct{})
	var wg sync.WaitGroup

	load := func(context.Context) ([]Place, error) {
		loads.Add(1)
		<-start
		return []Place{{PlaceID: "p1", NameEn: "One"}}, nil
	}

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := cache.getOrLoad(context.Background(), load)
			r.NoError(err)
		}()
	}

	time.Sleep(20 * time.Millisecond)
	close(start)
	wg.Wait()

	r.Equal(int32(1), loads.Load(), "concurrent cache misses must coalesce to one load")
}

func TestListMapCache_PeekHitsWithoutLoad(t *testing.T) {
	r := require.New(t)
	cache := newListMapCache(time.Minute)

	var loads atomic.Int32
	_, _, err := cache.getOrLoad(context.Background(), func(context.Context) ([]Place, error) {
		loads.Add(1)
		return []Place{{PlaceID: "p1"}}, nil
	})
	r.NoError(err)
	r.Equal(int32(1), loads.Load())

	_, etag, ok := cache.peek()
	r.True(ok)
	r.NotEmpty(etag)
	r.Equal(int32(1), loads.Load(), "peek must not trigger load")
}

func TestListMapCache_InvalidateClearsEntry(t *testing.T) {
	r := require.New(t)
	cache := newListMapCache(time.Minute)

	var loads atomic.Int32
	load := func(context.Context) ([]Place, error) {
		loads.Add(1)
		return []Place{{PlaceID: "p1"}}, nil
	}

	_, _, err := cache.getOrLoad(context.Background(), load)
	r.NoError(err)
	cache.invalidate()
	_, _, ok := cache.peek()
	r.False(ok)

	_, _, err = cache.getOrLoad(context.Background(), load)
	r.NoError(err)
	r.Equal(int32(2), loads.Load())
}
