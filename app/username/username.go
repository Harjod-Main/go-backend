package username

import (
	"regexp"
	"strings"
)

// Shared by profile updates and referral invite lookup.
var pattern = regexp.MustCompile(`^[a-zA-Z0-9._-]{3,30}$`)

// Normalize trims space and reports whether the result is a valid username.
func Normalize(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if !pattern.MatchString(s) {
		return "", false
	}
	return s, true
}
