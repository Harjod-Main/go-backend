package notifications

type WebPushSubscriptionKeys struct {
	P256dh string `json:"p256dh"`
	Auth   string `json:"auth"`
}

type WebPushSubscriptionRequest struct {
	Endpoint string                  `json:"endpoint"`
	Keys     WebPushSubscriptionKeys `json:"keys"`
}

type NotificationPreferencesRequest struct {
	NotificationsEnabled bool `json:"notificationsEnabled"`
	InAppAlertsEnabled   bool `json:"inAppAlertsEnabled"`
	InAppSoundsEnabled   bool `json:"inAppSoundsEnabled"`
}

type IOSPushTokenRequest struct {
	Token string `json:"token"`
}

type NotificationPreferences struct {
	NotificationsEnabled bool
	InAppAlertsEnabled   bool
	InAppSoundsEnabled   bool
}

// NotificationEvent is a high-level event from the app domain (check-in, review, submission, privilege).
// sender.go maps this into concrete push payloads.
type NotificationEvent struct {
	Type          string
	PlaceID       string
	Title         string
	Body          string
	URL           string
	PointsAwarded int
}

