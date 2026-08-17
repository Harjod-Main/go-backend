package notifications

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/RinTanth/go-backend/app/auth/supabaseauth"
	"github.com/RinTanth/go-common/app"
	"github.com/RinTanth/go-common/wrapper"
	"github.com/gin-gonic/gin"
)

const (
	maxNotificationBodyBytes = 16 * 1024
	maxWebPushEndpointLen    = 2048
	maxWebPushKeyLen         = 256
	maxIOSPushTokenLen       = 512
)

type HandlerConfig struct {
	Repo NotificationsRepository
}

type Handler struct {
	repo NotificationsRepository
}

func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{repo: cfg.Repo}
}

type genericOK struct {
	OK bool `json:"ok"`
}

func limitNotificationBody(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxNotificationBodyBytes)
}

func respondNotificationBadRequest(c *gin.Context) {
	wrapper.Respond(c, wrapper.ResponseOption[genericOK]{
		HTTPStatus: http.StatusBadRequest,
		Code:       app.CodeBadRequest,
		Message:    app.MessageBadRequest,
	})
}

func withinLen(value string, maxLen int) bool {
	return len(value) > 0 && len(value) <= maxLen
}

// PATCH /api/v1/me/notification-preferences
func (h *Handler) UpdatePreferences(c *gin.Context) {
	claims, ok := supabaseauth.ClaimsFromGin(c)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[genericOK]{
			HTTPStatus: http.StatusUnauthorized,
			Code:       app.CodeUnauthorized,
			Message:    app.MessageUnauthorized,
		})
		return
	}

	limitNotificationBody(c)
	var req NotificationPreferencesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondNotificationBadRequest(c)
		return
	}

	if err := h.repo.UpsertPreferences(c.Request.Context(), claims.Sub, req); err != nil {
		slog.Error("update notification preferences failed", "user_id", claims.Sub, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[genericOK]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}

	wrapper.Respond(c, wrapper.ResponseOption[genericOK]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       &genericOK{OK: true},
	})
}

// POST /api/v1/me/web-push-subscriptions
func (h *Handler) UpsertWebPushSubscription(c *gin.Context) {
	claims, ok := supabaseauth.ClaimsFromGin(c)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[genericOK]{
			HTTPStatus: http.StatusUnauthorized,
			Code:       app.CodeUnauthorized,
			Message:    app.MessageUnauthorized,
		})
		return
	}

	limitNotificationBody(c)
	var req WebPushSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondNotificationBadRequest(c)
		return
	}

	req.Endpoint = strings.TrimSpace(req.Endpoint)
	req.Keys.P256dh = strings.TrimSpace(req.Keys.P256dh)
	req.Keys.Auth = strings.TrimSpace(req.Keys.Auth)
	if !withinLen(req.Endpoint, maxWebPushEndpointLen) ||
		!withinLen(req.Keys.P256dh, maxWebPushKeyLen) ||
		!withinLen(req.Keys.Auth, maxWebPushKeyLen) {
		respondNotificationBadRequest(c)
		return
	}

	// Ensure JSON serializable keys (defensive).
	if _, err := json.Marshal(req.Keys); err != nil {
		respondNotificationBadRequest(c)
		return
	}

	if err := h.repo.UpsertWebPushSubscription(c.Request.Context(), claims.Sub, req); err != nil {
		slog.Error("upsert web push subscription failed", "user_id", claims.Sub, "endpoint", req.Endpoint, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[genericOK]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}

	wrapper.Respond(c, wrapper.ResponseOption[genericOK]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       &genericOK{OK: true},
	})
}

type deleteWebPushBody struct {
	Endpoint string `json:"endpoint"`
}

// DELETE /api/v1/me/web-push-subscriptions (supports optional JSON body { endpoint })
func (h *Handler) DeleteWebPushSubscription(c *gin.Context) {
	claims, ok := supabaseauth.ClaimsFromGin(c)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[genericOK]{
			HTTPStatus: http.StatusUnauthorized,
			Code:       app.CodeUnauthorized,
			Message:    app.MessageUnauthorized,
		})
		return
	}

	// Accept endpoint either from query or JSON body.
	limitNotificationBody(c)
	endpoint := strings.TrimSpace(c.Query("endpoint"))
	if endpoint == "" {
		var body deleteWebPushBody
		if err := c.ShouldBindJSON(&body); err == nil {
			endpoint = strings.TrimSpace(body.Endpoint)
		}
	}

	if !withinLen(endpoint, maxWebPushEndpointLen) {
		respondNotificationBadRequest(c)
		return
	}

	if err := h.repo.DeleteWebPushSubscription(c.Request.Context(), claims.Sub, endpoint); err != nil {
		slog.Error("delete web push subscription failed", "user_id", claims.Sub, "endpoint", endpoint, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[genericOK]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}

	wrapper.Respond(c, wrapper.ResponseOption[genericOK]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       &genericOK{OK: true},
	})
}

// DELETE compatibility with older temporary route used by the frontend while backend is being built.
func (h *Handler) DeleteWebPushSubscriptionCompat(c *gin.Context) {
	h.DeleteWebPushSubscription(c)
}

// POST /api/v1/me/ios-push-token
func (h *Handler) UpsertIOSPushToken(c *gin.Context) {
	claims, ok := supabaseauth.ClaimsFromGin(c)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[genericOK]{
			HTTPStatus: http.StatusUnauthorized,
			Code:       app.CodeUnauthorized,
			Message:    app.MessageUnauthorized,
		})
		return
	}

	limitNotificationBody(c)
	var req IOSPushTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondNotificationBadRequest(c)
		return
	}

	req.Token = strings.TrimSpace(req.Token)
	if !withinLen(req.Token, maxIOSPushTokenLen) {
		respondNotificationBadRequest(c)
		return
	}

	if err := h.repo.UpsertIOSPushToken(c.Request.Context(), claims.Sub, req); err != nil {
		slog.Error("upsert ios push token failed", "user_id", claims.Sub, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[genericOK]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}

	wrapper.Respond(c, wrapper.ResponseOption[genericOK]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       &genericOK{OK: true},
	})
}

// DELETE /api/v1/me/ios-push-token?token=...
type deleteIOSPushBody struct {
	Token string `json:"token"`
}

func (h *Handler) DeleteIOSPushToken(c *gin.Context) {
	claims, ok := supabaseauth.ClaimsFromGin(c)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[genericOK]{
			HTTPStatus: http.StatusUnauthorized,
			Code:       app.CodeUnauthorized,
			Message:    app.MessageUnauthorized,
		})
		return
	}

	limitNotificationBody(c)
	token := strings.TrimSpace(c.Query("token"))
	if token == "" {
		var body deleteIOSPushBody
		if err := c.ShouldBindJSON(&body); err == nil {
			token = strings.TrimSpace(body.Token)
		}
	}

	if !withinLen(token, maxIOSPushTokenLen) {
		respondNotificationBadRequest(c)
		return
	}

	if err := h.repo.DeleteIOSPushToken(c.Request.Context(), claims.Sub, token); err != nil {
		slog.Error("delete ios push token failed", "user_id", claims.Sub, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[genericOK]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}

	wrapper.Respond(c, wrapper.ResponseOption[genericOK]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       &genericOK{OK: true},
	})
}

// GET /api/v1/me/notification-preferences
func (h *Handler) GetPreferences(c *gin.Context) {
	claims, ok := supabaseauth.ClaimsFromGin(c)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[genericOK]{
			HTTPStatus: http.StatusUnauthorized,
			Code:       app.CodeUnauthorized,
			Message:    app.MessageUnauthorized,
		})
		return
	}

	prefs, err := h.repo.GetPreferences(c.Request.Context(), claims.Sub)
	if err != nil {
		slog.Error("get notification preferences failed", "user_id", claims.Sub, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[genericOK]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}

	wrapper.Respond(c, wrapper.ResponseOption[NotificationPreferences]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       prefs,
	})
}
