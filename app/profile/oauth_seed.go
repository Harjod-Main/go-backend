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

func isEmailDerivedDisplayName(name, email string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(name))
	if trimmed == "" || isGenericDisplayName(trimmed) {
		return false
	}
	local := strings.TrimSpace(strings.ToLower(defaultDisplayName(email)))
	return local != "" && !isGenericDisplayName(local) && trimmed == local
}

func shouldBackfillDisplayName(existingName, email, seedName string) bool {
	seedName = strings.TrimSpace(seedName)
	if seedName == "" || isGenericDisplayName(seedName) || isEmailDerivedDisplayName(seedName, email) {
		return false
	}
	existingName = strings.TrimSpace(existingName)
	return isGenericDisplayName(existingName) || isEmailDerivedDisplayName(existingName, email)
}

func isGenericUsername(username string) bool {
	switch strings.TrimSpace(strings.ToLower(username)) {
	case "", "user":
		return true
	default:
		return false
	}
}
