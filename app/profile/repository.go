package profile

import (
	"context"
	"errors"
)

var ErrUsernameTaken = errors.New("username already taken")
var ErrNotFound = errors.New("profile not found")

type Repository interface {
	GetByUserID(ctx context.Context, userID string) (*Profile, error)
	Ensure(ctx context.Context, userID, email string) (*Profile, error)
	Update(ctx context.Context, userID string, displayName, username *string, avatarURL *string, clearAvatar bool) (*Profile, error)
}
