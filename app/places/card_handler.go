package places

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/RinTanth/go-common/app"
	"github.com/RinTanth/go-common/wrapper"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// GetMapPlaceCard returns weekly hours, gallery photos, and entrances for a selected pin.
func (h *Handler) GetMapPlaceCard(c *gin.Context) {
	placeID := strings.TrimSpace(c.Param("placeId"))
	if _, err := uuid.Parse(placeID); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[MapPlaceCard]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	card, err := h.repo.GetMapPlaceCard(c.Request.Context(), placeID)
	if err != nil {
		slog.Error("map place card failed", "place_id", placeID, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[MapPlaceCard]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}
	if card == nil {
		wrapper.Respond(c, wrapper.ResponseOption[MapPlaceCard]{
			HTTPStatus: http.StatusNotFound,
			Code:       app.CodeNotFound,
			Message:    app.MessageNotFound,
		})
		return
	}

	wrapper.Respond(c, wrapper.ResponseOption[MapPlaceCard]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       card,
	})
}
