package profile

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShouldBackfillDisplayName_EmailDerivedExisting(t *testing.T) {
	r := require.New(t)
	seed := OAuthSeedFromMetadata("aif912752@gmail.com", map[string]any{
		"full_name": "Suer Jet",
	})
	r.True(shouldBackfillDisplayName("aif912752", "aif912752@gmail.com", seed.DisplayName))
}

func TestShouldBackfillDisplayName_KeepsCustomName(t *testing.T) {
	r := require.New(t)
	seed := OAuthSeedFromMetadata("aif912752@gmail.com", map[string]any{
		"full_name": "Suer Jet",
	})
	r.False(shouldBackfillDisplayName("Johnny", "aif912752@gmail.com", seed.DisplayName))
}
