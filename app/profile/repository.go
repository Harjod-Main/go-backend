package profile

import (
	"context"
	"errors"
)

var (
	ErrUsernameTaken      = errors.New("username already taken")
	ErrNotFound           = errors.New("profile not found")
	ErrInvalidDisplayName = errors.New("invalid display name")
	ErrInvalidUsername    = errors.New("invalid username")
	ErrInvalidAvatarURL   = errors.New("invalid avatar url")
)

func IsValidationError(err error) bool {
	return errors.Is(err, ErrInvalidDisplayName) ||
		errors.Is(err, ErrInvalidUsername) ||
		errors.Is(err, ErrInvalidAvatarURL)
}

type Repository interface {
	GetByUserID(ctx context.Context, userID string) (*Profile, error)
	Ensure(ctx context.Context, userID, email string, seed OAuthSeed) (*Profile, error)
	// SyncFromOAuth backfills OAuth display name / avatar onto an existing profile.
	// It never creates a row; returns ErrNotFound when the profile is missing.
	SyncFromOAuth(ctx context.Context, userID, email string, seed OAuthSeed) (*Profile, error)
	Update(ctx context.Context, userID string, displayName, username *string, avatarURL *string, clearAvatar bool) (*Profile, error)
}
