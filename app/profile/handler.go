package profile

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/RinTanth/go-backend/app/auth/supabaseauth"
	"github.com/RinTanth/go-backend/app/pagination"
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

	p, err := h.repo.GetByUserID(c.Request.Context(), claims.Sub)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			wrapper.Respond(c, wrapper.ResponseOption[Profile]{
				HTTPStatus: http.StatusNotFound,
				Code:       app.CodeNotFound,
				Message:    app.MessageNotFound,
			})
			return
		}
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

// ListLeaderboard handles GET /api/v1/leaderboard?limit=
func (h *Handler) ListLeaderboard(c *gin.Context) {
	limit := pagination.ParseLimit(c.Query("limit"), 20, 100)

	entries, err := h.repo.ListLeaderboard(c.Request.Context(), limit)
	if err != nil {
		slog.Error("list leaderboard failed", "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[LeaderboardResponse]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}
	if entries == nil {
		entries = []LeaderboardEntry{}
	}

	resp := LeaderboardResponse{Entries: entries}
	if claims, ok := supabaseauth.ClaimsFromGin(c); ok {
		rank, creditPoints, rankErr := h.repo.LeaderboardRank(c.Request.Context(), claims.Sub)
		if rankErr == nil {
			resp.Self = &LeaderboardSelf{Rank: rank, CreditPoints: creditPoints}
		} else if !errors.Is(rankErr, ErrNotFound) {
			slog.Error("leaderboard self rank failed", "user_id", claims.Sub, "error", rankErr)
		}
	}

	wrapper.Respond(c, wrapper.ResponseOption[LeaderboardResponse]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       &resp,
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
	seed := OAuthSeedFromMetadata(claims.Email, claims.UserMetadata)
	if _, err := h.repo.Ensure(c.Request.Context(), claims.Sub, claims.Email, seed); err != nil {
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
