package notifications_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/RinTanth/go-backend/app/auth/supabaseauth"
	"github.com/RinTanth/go-backend/app/notifications"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type stubRepo struct {
	upsertWebPushCalled bool
	upsertIOSCalled     bool
	upsertPrefsCalled   bool
	lastEndpoint        string
	lastToken           string
}

func (s *stubRepo) UpsertPreferences(context.Context, string, notifications.NotificationPreferencesRequest) error {
	s.upsertPrefsCalled = true
	return nil
}
func (s *stubRepo) GetPreferences(context.Context, string) (*notifications.NotificationPreferences, error) {
	return &notifications.NotificationPreferences{}, nil
}
func (s *stubRepo) UpsertWebPushSubscription(_ context.Context, _ string, req notifications.WebPushSubscriptionRequest) error {
	s.upsertWebPushCalled = true
	s.lastEndpoint = req.Endpoint
	return nil
}
func (s *stubRepo) DeleteWebPushSubscription(context.Context, string, string) error {
	return nil
}
func (s *stubRepo) UpsertIOSPushToken(_ context.Context, _ string, req notifications.IOSPushTokenRequest) error {
	s.upsertIOSCalled = true
	s.lastToken = req.Token
	return nil
}
func (s *stubRepo) DeleteIOSPushToken(context.Context, string, string) error { return nil }
func (s *stubRepo) ListWebPushSubscriptions(context.Context, string) ([]notifications.WebPushSubscriptionRequest, error) {
	return nil, nil
}
func (s *stubRepo) ListIOSPushTokens(context.Context, string) ([]notifications.IOSPushTokenRequest, error) {
	return nil, nil
}

func withClaims(handler gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(supabaseauth.CtxClaimsKey, &supabaseauth.Claims{
			Sub:   "11111111-1111-1111-1111-111111111111",
			Email: "user@example.com",
		})
		handler(c)
	}
}

func perform(t *testing.T, method, path string, handler gin.HandlerFunc, body any) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	route := path
	if i := strings.IndexByte(path, '?'); i >= 0 {
		route = path[:i]
	}
	engine.Handle(method, route, withClaims(handler))

	var payload []byte
	switch v := body.(type) {
	case nil:
	case string:
		payload = []byte(v)
	default:
		raw, err := json.Marshal(v)
		require.NoError(t, err)
		payload = raw
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func TestUpdatePreferences_RejectsOversizedBody(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{}
	h := notifications.NewHandler(notifications.HandlerConfig{Repo: repo})
	w := perform(t, http.MethodPatch, "/api/v1/me/notification-preferences", h.UpdatePreferences, map[string]any{
		"notificationsEnabled": true,
		"padding":              strings.Repeat("x", 20*1024),
	})
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(repo.upsertPrefsCalled)
}

func TestUpsertWebPush_RejectsOversizedBody(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{}
	h := notifications.NewHandler(notifications.HandlerConfig{Repo: repo})
	w := perform(t, http.MethodPost, "/api/v1/me/web-push-subscriptions", h.UpsertWebPushSubscription, map[string]any{
		"endpoint": "https://example.com/push",
		"keys": map[string]string{
			"p256dh":  "abc",
			"auth":    "def",
			"padding": strings.Repeat("x", 20*1024),
		},
	})
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(repo.upsertWebPushCalled)
}

func TestUpsertWebPush_RejectsLongEndpointAndKeys(t *testing.T) {
	r := require.New(t)

	longEndpoint := &stubRepo{}
	h := notifications.NewHandler(notifications.HandlerConfig{Repo: longEndpoint})
	w := perform(t, http.MethodPost, "/api/v1/me/web-push-subscriptions", h.UpsertWebPushSubscription, map[string]any{
		"endpoint": "https://example.com/" + strings.Repeat("a", 2048),
		"keys":     map[string]string{"p256dh": "abc", "auth": "def"},
	})
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(longEndpoint.upsertWebPushCalled)

	longKey := &stubRepo{}
	h = notifications.NewHandler(notifications.HandlerConfig{Repo: longKey})
	w = perform(t, http.MethodPost, "/api/v1/me/web-push-subscriptions", h.UpsertWebPushSubscription, map[string]any{
		"endpoint": "https://example.com/push",
		"keys":     map[string]string{"p256dh": strings.Repeat("a", 257), "auth": "def"},
	})
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(longKey.upsertWebPushCalled)
}

func TestUpsertWebPush_AcceptsValidBody(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{}
	h := notifications.NewHandler(notifications.HandlerConfig{Repo: repo})
	w := perform(t, http.MethodPost, "/api/v1/me/web-push-subscriptions", h.UpsertWebPushSubscription, map[string]any{
		"endpoint": "https://fcm.googleapis.com/fcm/send/abc",
		"keys":     map[string]string{"p256dh": "p256dh-key", "auth": "auth-key"},
	})
	r.Equal(http.StatusOK, w.Code)
	r.True(repo.upsertWebPushCalled)
	r.Equal("https://fcm.googleapis.com/fcm/send/abc", repo.lastEndpoint)
}

func TestUpsertIOSPushToken_RejectsLongTokenAndOversizedBody(t *testing.T) {
	r := require.New(t)

	longToken := &stubRepo{}
	h := notifications.NewHandler(notifications.HandlerConfig{Repo: longToken})
	w := perform(t, http.MethodPost, "/api/v1/me/ios-push-token", h.UpsertIOSPushToken, map[string]any{
		"token": strings.Repeat("t", 513),
	})
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(longToken.upsertIOSCalled)

	oversized := &stubRepo{}
	h = notifications.NewHandler(notifications.HandlerConfig{Repo: oversized})
	w = perform(t, http.MethodPost, "/api/v1/me/ios-push-token", h.UpsertIOSPushToken, map[string]any{
		"token":   "ExponentPushToken[abc]",
		"padding": strings.Repeat("x", 20*1024),
	})
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(oversized.upsertIOSCalled)
}

func TestDeleteWebPush_RejectsLongEndpoint(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{}
	h := notifications.NewHandler(notifications.HandlerConfig{Repo: repo})
	w := perform(
		t,
		http.MethodDelete,
		"/api/v1/me/web-push-subscriptions?endpoint="+strings.Repeat("a", 2049),
		h.DeleteWebPushSubscription,
		nil,
	)
	r.Equal(http.StatusBadRequest, w.Code)
}
