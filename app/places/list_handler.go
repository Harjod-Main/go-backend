package places

import (
	"log/slog"
	"net/http"

	"github.com/RinTanth/go-common/app"
	"github.com/RinTanth/go-common/wrapper"
	"github.com/gin-gonic/gin"
)

// List returns non-blacklisted places for the map drawer (public read).
func (h *Handler) List(c *gin.Context) {
	places, err := h.repo.ListMapPlaces(c.Request.Context())
	if err != nil {
		slog.Error("places list failed", "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[[]Place]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}

	wrapper.Respond(c, wrapper.ResponseOption[[]Place]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       &places,
	})
}
