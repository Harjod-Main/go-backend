package referrals

import (
	"context"
	"errors"
	"time"
)

const MaxAccountAge = 7 * 24 * time.Hour

var (
	ErrInvalidUsername  = errors.New("invalid invite username")
	ErrReferrerNotFound = errors.New("referrer not found")
	ErrRefereeNotFound  = errors.New("referee profile not found")
	ErrSelfReferral     = errors.New("cannot use your own invite")
	ErrAlreadyReferred  = errors.New("already referred")
	ErrNotEligible      = errors.New("referral is only for new accounts")
)

type Repository interface {
	Accept(ctx context.Context, in AcceptInput) (*AcceptOutcome, error)
}
