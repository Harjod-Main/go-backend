package places

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseMapBounds_EmptyIsUnbounded(t *testing.T) {
	r := require.New(t)
	bounds, err := parseMapBounds("", "", "", "")
	r.NoError(err)
	r.Nil(bounds)
}

func TestParseMapBounds_RequiresAllFour(t *testing.T) {
	r := require.New(t)
	_, err := parseMapBounds("100.4", "13.7", "100.6", "")
	r.Error(err)
}

func TestParseMapBounds_RejectsInvertedAndHuge(t *testing.T) {
	r := require.New(t)
	_, err := parseMapBounds("100.6", "13.7", "100.4", "13.8")
	r.Error(err)
	_, err = parseMapBounds("100.0", "13.0", "103.0", "13.5")
	r.Error(err)
}

func TestParseMapBounds_AcceptsBangkokViewport(t *testing.T) {
	r := require.New(t)
	bounds, err := parseMapBounds("100.45", "13.70", "100.60", "13.80")
	r.NoError(err)
	r.Equal(100.45, bounds.West)
	r.Equal(13.70, bounds.South)
	r.Equal(100.60, bounds.East)
	r.Equal(13.80, bounds.North)
}
