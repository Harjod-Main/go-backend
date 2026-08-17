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
	NotificationsEnabled bool `json:"notificationsEnabled"`
	InAppAlertsEnabled   bool `json:"inAppAlertsEnabled"`
	InAppSoundsEnabled   bool `json:"inAppSoundsEnabled"`
}

// NotificationEvent is a high-level event from the app domain (check-in, review, submission, privilege).
// sender.go maps this into concrete push payloads.
type NotificationEvent struct {
	Type          string `json:"type"`
	PlaceID       string `json:"placeId,omitempty"`
	Title         string `json:"title"`
	Body          string `json:"body"`
	URL           string `json:"url"`
	PointsAwarded int    `json:"pointsAwarded,omitempty"`
}

// NotificationJob is a durable push delivery attempt. Handlers enqueue and return;
// a worker sends Web Push / Expo Push with retries.
type NotificationJob struct {
	JobID       string
	UserID      string
	Event       NotificationEvent
	Attempts    int
	MaxAttempts int
}
