package notifications

import (
	"context"
)

type NotificationPreferencesRepository interface {
	UpsertPreferences(ctx context.Context, userID string, req NotificationPreferencesRequest) error
}

type PushTokensRepository interface {
	UpsertWebPushSubscription(ctx context.Context, userID string, req WebPushSubscriptionRequest) error
	DeleteWebPushSubscription(ctx context.Context, userID string, endpoint string) error

	UpsertIOSPushToken(ctx context.Context, userID string, req IOSPushTokenRequest) error
	DeleteIOSPushToken(ctx context.Context, userID string, token string) error

	GetPreferences(ctx context.Context, userID string) (*NotificationPreferences, error)
	ListWebPushSubscriptions(ctx context.Context, userID string) ([]WebPushSubscriptionRequest, error)
	ListIOSPushTokens(ctx context.Context, userID string) ([]IOSPushTokenRequest, error)
}

// Optional: store tokens for future sender implementation.
type NotificationsRepository interface {
	NotificationPreferencesRepository
	PushTokensRepository
}

