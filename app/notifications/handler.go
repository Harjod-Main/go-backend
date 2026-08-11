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

	var req NotificationPreferencesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[genericOK]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
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

	var req WebPushSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[genericOK]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	req.Endpoint = strings.TrimSpace(req.Endpoint)
	if req.Endpoint == "" {
		wrapper.Respond(c, wrapper.ResponseOption[genericOK]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}
	if req.Keys.P256dh == "" || req.Keys.Auth == "" {
		wrapper.Respond(c, wrapper.ResponseOption[genericOK]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	// Ensure JSON serializable keys (defensive).
	if _, err := json.Marshal(req.Keys); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[genericOK]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
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
	endpoint := strings.TrimSpace(c.Query("endpoint"))
	if endpoint == "" {
		var body deleteWebPushBody
		if err := c.ShouldBindJSON(&body); err == nil {
			endpoint = strings.TrimSpace(body.Endpoint)
		}
	}

	if endpoint == "" {
		wrapper.Respond(c, wrapper.ResponseOption[genericOK]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
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

	var req IOSPushTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[genericOK]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" {
		wrapper.Respond(c, wrapper.ResponseOption[genericOK]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
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

	token := strings.TrimSpace(c.Query("token"))
	if token == "" {
		var body deleteIOSPushBody
		if err := c.ShouldBindJSON(&body); err == nil {
			token = strings.TrimSpace(body.Token)
		}
	}

	if token == "" {
		wrapper.Respond(c, wrapper.ResponseOption[genericOK]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
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

