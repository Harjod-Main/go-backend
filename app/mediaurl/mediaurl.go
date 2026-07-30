package mediaurl

import (
	"strings"
	"sync"
)

const MaxURLLen = 2048

var (
	mu             sync.RWMutex
	allowedPrefix  string
	configuredOnce bool
)

// Configure sets the only allowed public media URL prefix from SUPABASE_PROJECT_URL.
// Example prefix: https://xxxx.supabase.co/storage/v1/object/public/media/
func Configure(projectURL string) {
	base := strings.TrimRight(strings.TrimSpace(projectURL), "/")
	mu.Lock()
	defer mu.Unlock()
	if base == "" {
		allowedPrefix = ""
		configuredOnce = true
		return
	}
	allowedPrefix = base + "/storage/v1/object/public/media/"
	configuredOnce = true
}

// PublicPrefix returns the configured media public URL prefix (may be empty if not configured).
func PublicPrefix() string {
	mu.RLock()
	defer mu.RUnlock()
	return allowedPrefix
}

// ResetForTest clears configuration (tests only).
func ResetForTest() {
	mu.Lock()
	defer mu.Unlock()
	allowedPrefix = ""
	configuredOnce = false
}

func currentPrefix() (string, bool) {
	mu.RLock()
	defer mu.RUnlock()
	return allowedPrefix, configuredOnce && allowedPrefix != ""
}

// ValidMediaURLs returns true when every item is a public URL under our media bucket.
// Empty slices are valid. Rejects file://, foreign hosts, and path traversal.
func ValidMediaURLs(urls []string, maxLen int) bool {
	if maxLen <= 0 {
		maxLen = MaxURLLen
	}
	prefix, ok := currentPrefix()
	if !ok {
		// Fail closed until Configure() runs at boot.
		return len(urls) == 0
	}
	for _, item := range urls {
		if !validOneMediaURL(strings.TrimSpace(item), prefix, maxLen) {
			return false
		}
	}
	return true
}

func validOneMediaURL(trimmed, prefix string, maxLen int) bool {
	if trimmed == "" || len(trimmed) > maxLen {
		return false
	}
	if !strings.HasPrefix(trimmed, prefix) {
		return false
	}
	rest := strings.TrimPrefix(trimmed, prefix)
	if rest == "" || strings.Contains(rest, "..") || strings.Contains(rest, "\\") {
		return false
	}
	// Disallow absolute-looking remnants and query/fragment tricks that change the object identity.
	if strings.HasPrefix(rest, "/") || strings.ContainsAny(rest, "?#") {
		return false
	}
	return true
}

// ValidAvatarValue accepts preset keys (preset:N) or our media-bucket public URLs.
func ValidAvatarValue(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "preset:") {
		return len(trimmed) <= 32
	}
	return ValidMediaURLs([]string{trimmed}, MaxURLLen)
}
