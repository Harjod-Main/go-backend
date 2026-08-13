package mystats

import (
	"log/slog"
	"net/http"

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

// GetMine handles GET /api/v1/me/stats (auth required).
func (h *Handler) GetMine(c *gin.Context) {
	claims, ok := supabaseauth.ClaimsFromGin(c)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[Stats]{
			HTTPStatus: http.StatusUnauthorized,
			Code:       app.CodeUnauthorized,
			Message:    app.MessageUnauthorized,
		})
		return
	}

	stats, err := h.repo.CountByUser(c.Request.Context(), claims.Sub)
	if err != nil {
		slog.Error("count my stats failed", "user_id", claims.Sub, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[Stats]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}
	if stats == nil {
		stats = &Stats{}
	}

	wrapper.Respond(c, wrapper.ResponseOption[Stats]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       stats,
	})
}
