package places

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	listMapPlacesCacheTTL     = 30 * time.Second
	listMapPlacesCacheControl = "public, max-age=30"
)

type cacheEntry struct {
	places []Place
	etag   string
}

// listMapCache memoizes ListMapPlaces for a short TTL to avoid repeating the
// heavy nested aggregation on every public GET.
type listMapCache struct {
	mu      sync.RWMutex
	ttl     time.Duration
	places  []Place
	etag    string
	expires time.Time
	load    singleflight.Group
}

func newListMapCache(ttl time.Duration) *listMapCache {
	if ttl <= 0 {
		ttl = listMapPlacesCacheTTL
	}
	return &listMapCache{ttl: ttl}
}

// peek returns the current cache entry without loading. The bool is true when
// the entry is still within TTL (suitable for fast 304 / cache hits).
func (c *listMapCache) peek() ([]Place, string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.places != nil && time.Now().Before(c.expires) {
		return c.places, c.etag, true
	}
	return nil, "", false
}

func (c *listMapCache) getOrLoad(ctx context.Context, load func(context.Context) ([]Place, error)) ([]Place, string, error) {
	if places, etag, ok := c.peek(); ok {
		return places, etag, nil
	}

	result, err, _ := c.load.Do("list", func() (any, error) {
		if places, etag, ok := c.peek(); ok {
			return cacheEntry{places: places, etag: etag}, nil
		}

		// Detach from the caller request so one cancelled client does not abort
		// the shared refresh for concurrent waiters.
		places, err := load(context.WithoutCancel(ctx))
		if err != nil {
			return nil, err
		}

		etag, err := etagForPlaces(places)
		if err != nil {
			return nil, err
		}

		c.mu.Lock()
		c.places = places
		c.etag = etag
		c.expires = time.Now().Add(c.ttl)
		c.mu.Unlock()

		return cacheEntry{places: places, etag: etag}, nil
	})
	if err != nil {
		return nil, "", err
	}

	entry := result.(cacheEntry)
	return entry.places, entry.etag, nil
}

func etagForPlaces(places []Place) (string, error) {
	raw, err := json.Marshal(places)
	if err != nil {
		return "", fmt.Errorf("etag map places: %w", err)
	}
	sum := sha1.Sum(raw)
	return `"` + hex.EncodeToString(sum[:]) + `"`, nil
}

func etagMatches(ifNoneMatch, etag string) bool {
	if ifNoneMatch == "" || etag == "" {
		return false
	}
	for _, part := range strings.Split(ifNoneMatch, ",") {
		candidate := strings.TrimSpace(part)
		candidate = strings.TrimPrefix(candidate, "W/")
		if candidate == "*" || candidate == etag {
			return true
		}
	}
	return false
}
