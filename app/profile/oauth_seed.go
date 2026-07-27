package profile

import (
	"strings"
)

// OAuthSeed carries identity hints from Supabase Auth JWT user_metadata.
type OAuthSeed struct {
	DisplayName string
	AvatarURL   *string
}

func OAuthSeedFromMetadata(email string, meta map[string]any) OAuthSeed {
	seed := OAuthSeed{
		DisplayName: nameFromMetadata(meta),
	}
	if seed.DisplayName == "" {
		seed.DisplayName = defaultDisplayName(email)
	}
	if url := avatarFromMetadata(meta); url != "" {
		seed.AvatarURL = &url
	}
	return seed
}

func nameFromMetadata(meta map[string]any) string {
	if meta == nil {
		return ""
	}
	for _, key := range []string{"full_name", "name", "display_name", "displayName", "nickname"} {
		if s, ok := meta[key].(string); ok {
			if trimmed := strings.TrimSpace(s); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func avatarFromMetadata(meta map[string]any) string {
	if meta == nil {
		return ""
	}
	for _, key := range []string{"avatar_url", "picture", "avatar"} {
		if s, ok := meta[key].(string); ok {
			if trimmed := strings.TrimSpace(s); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func isGenericDisplayName(name string) bool {
	switch strings.TrimSpace(strings.ToLower(name)) {
	case "", "user":
		return true
	default:
		return false
	}
}

func isGenericUsername(username string) bool {
	switch strings.TrimSpace(strings.ToLower(username)) {
	case "", "user":
		return true
	default:
		return false
	}
}
