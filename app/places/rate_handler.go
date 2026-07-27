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

// GetRate returns the parking rate sheet for a place (public read).
func (h *Handler) GetRate(c *gin.Context) {
	placeID := strings.TrimSpace(c.Param("placeId"))
	if _, err := uuid.Parse(placeID); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[PlaceRateDetail]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	rate, err := h.repo.GetPlaceRate(c.Request.Context(), placeID)
	if err != nil {
		slog.Error("place rate failed", "place_id", placeID, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[PlaceRateDetail]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}

	wrapper.Respond(c, wrapper.ResponseOption[PlaceRateDetail]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       rate,
	})
}
