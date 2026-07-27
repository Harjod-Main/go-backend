package profile

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/RinTanth/go-backend/app/auth/supabaseauth"
	"github.com/RinTanth/go-common/app"
	"github.com/RinTanth/go-common/wrapper"
	"github.com/gin-gonic/gin"
)

type HandlerConfig struct {
	Repo Repository
}

type Handler struct {
	repo Repository
}

func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{repo: cfg.Repo}
}

// Get handles GET /api/v1/profile
func (h *Handler) Get(c *gin.Context) {
	claims, ok := supabaseauth.ClaimsFromGin(c)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[Profile]{
			HTTPStatus: http.StatusUnauthorized,
			Code:       app.CodeUnauthorized,
			Message:    app.MessageUnauthorized,
		})
		return
	}

	p, err := h.repo.Ensure(c.Request.Context(), claims.Sub, claims.Email)
	if err != nil {
		slog.Error("get profile failed", "user_id", claims.Sub, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[Profile]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}

	wrapper.Respond(c, wrapper.ResponseOption[Profile]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       p,
	})
}

// Update handles PATCH /api/v1/profile
func (h *Handler) Update(c *gin.Context) {
	claims, ok := supabaseauth.ClaimsFromGin(c)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[Profile]{
			HTTPStatus: http.StatusUnauthorized,
			Code:       app.CodeUnauthorized,
			Message:    app.MessageUnauthorized,
		})
		return
	}

	var body UpdateProfileRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[Profile]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	if body.DisplayName == nil && body.Username == nil && body.AvatarURL == nil {
		wrapper.Respond(c, wrapper.ResponseOption[Profile]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	// Ensure row exists before update.
	if _, err := h.repo.Ensure(c.Request.Context(), claims.Sub, claims.Email); err != nil {
		slog.Error("ensure profile before update failed", "user_id", claims.Sub, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[Profile]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}

	clearAvatar := false
	if body.AvatarURL != nil && strings.TrimSpace(*body.AvatarURL) == "" {
		clearAvatar = true
	}

	p, err := h.repo.Update(c.Request.Context(), claims.Sub, body.DisplayName, body.Username, body.AvatarURL, clearAvatar)
	if err != nil {
		if errors.Is(err, ErrUsernameTaken) {
			wrapper.Respond(c, wrapper.ResponseOption[Profile]{
				HTTPStatus: http.StatusConflict,
				Code:       app.CodeBadRequest,
				Message:    "username already taken",
			})
			return
		}
		if IsValidationError(err) {
			wrapper.Respond(c, wrapper.ResponseOption[Profile]{
				HTTPStatus: http.StatusBadRequest,
				Code:       app.CodeBadRequest,
				Message:    app.MessageBadRequest,
			})
			return
		}
		slog.Error("update profile failed", "user_id", claims.Sub, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[Profile]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}

	wrapper.Respond(c, wrapper.ResponseOption[Profile]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       p,
	})
}
