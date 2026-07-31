package pagination_test

import (
	"testing"
	"time"

	"github.com/RinTanth/go-backend/app/pagination"
	"github.com/stretchr/testify/require"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	r := require.New(t)
	createdAt := time.Date(2026, 7, 31, 7, 0, 0, 123456789, time.UTC)
	id := "11111111-1111-1111-1111-111111111111"

	token := pagination.Encode(createdAt, id)
	got, err := pagination.Decode(token)
	r.NoError(err)
	r.True(got.CreatedAt.Equal(createdAt))
	r.Equal(id, got.ID)
}

func TestDecodeRejectsInvalid(t *testing.T) {
	r := require.New(t)
	_, err := pagination.Decode("")
	r.ErrorIs(err, pagination.ErrInvalidCursor)
	_, err = pagination.Decode("not-base64")
	r.ErrorIs(err, pagination.ErrInvalidCursor)
	_, err = pagination.Decode(pagination.Encode(time.Now().UTC(), "not-a-uuid"))
	r.ErrorIs(err, pagination.ErrInvalidCursor)
}

func TestParseLimit(t *testing.T) {
	r := require.New(t)
	r.Equal(20, pagination.ParseLimit("", 20, 100))
	r.Equal(20, pagination.ParseLimit("0", 20, 100))
	r.Equal(20, pagination.ParseLimit("101", 20, 100))
	r.Equal(50, pagination.ParseLimit("50", 20, 100))
}

func TestNextFromLast(t *testing.T) {
	r := require.New(t)
	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	id := "22222222-2222-2222-2222-222222222222"

	r.Nil(pagination.NextFromLast(false, createdAt, id))
	next := pagination.NextFromLast(true, createdAt, id)
	r.NotNil(next)
	got, err := pagination.Decode(*next)
	r.NoError(err)
	r.Equal(id, got.ID)
}
