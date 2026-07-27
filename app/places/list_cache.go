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
)

const (
	listMapPlacesCacheTTL     = 30 * time.Second
	listMapPlacesCacheControl = "public, max-age=30"
)

// listMapCache memoizes ListMapPlaces for a short TTL to avoid repeating the
// heavy nested aggregation on every public GET.
type listMapCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	places  []Place
	etag    string
	expires time.Time
}

func newListMapCache(ttl time.Duration) *listMapCache {
	if ttl <= 0 {
		ttl = listMapPlacesCacheTTL
	}
	return &listMapCache{ttl: ttl}
}

func (c *listMapCache) getOrLoad(ctx context.Context, load func(context.Context) ([]Place, error)) ([]Place, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	if c.places != nil && now.Before(c.expires) {
		return c.places, c.etag, nil
	}

	places, err := load(ctx)
	if err != nil {
		return nil, "", err
	}

	etag, err := etagForPlaces(places)
	if err != nil {
		return nil, "", err
	}

	c.places = places
	c.etag = etag
	c.expires = now.Add(c.ttl)
	return c.places, c.etag, nil
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
