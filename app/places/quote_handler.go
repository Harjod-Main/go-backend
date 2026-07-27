package places

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/RinTanth/go-common/app"
	"github.com/RinTanth/go-common/wrapper"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type quoteRequestBody struct {
	Hours    float64  `json:"hours"`
	PlaceIDs []string `json:"placeIds"`
}

// GetQuote returns a price quote for one place and stay duration (query: hours).
func (h *Handler) GetQuote(c *gin.Context) {
	placeID := strings.TrimSpace(c.Param("placeId"))
	if _, err := uuid.Parse(placeID); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[Quote]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	hours, err := strconv.ParseFloat(strings.TrimSpace(c.Query("hours")), 64)
	if err != nil || hours < 0 {
		wrapper.Respond(c, wrapper.ResponseOption[Quote]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	rate, err := h.repo.GetPlaceRate(c.Request.Context(), placeID)
	if err != nil {
		slog.Error("quote rate lookup failed", "place_id", placeID, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[Quote]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}

	quote := CalculateQuote(placeID, hours, rate)
	wrapper.Respond(c, wrapper.ResponseOption[Quote]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       &quote,
	})
}

// CreateQuotes returns price quotes for many places at once.
// Rates are loaded in a single batch query (not N sequential round-trips).
func (h *Handler) CreateQuotes(c *gin.Context) {
	var body quoteRequestBody
	if err := c.ShouldBindJSON(&body); err != nil || body.Hours < 0 || len(body.PlaceIDs) == 0 {
		wrapper.Respond(c, wrapper.ResponseOption[[]Quote]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}
	if len(body.PlaceIDs) > 100 {
		wrapper.Respond(c, wrapper.ResponseOption[[]Quote]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	placeIDs := make([]string, 0, len(body.PlaceIDs))
	for _, placeID := range body.PlaceIDs {
		placeID = strings.TrimSpace(placeID)
		if _, err := uuid.Parse(placeID); err != nil {
			wrapper.Respond(c, wrapper.ResponseOption[[]Quote]{
				HTTPStatus: http.StatusBadRequest,
				Code:       app.CodeBadRequest,
				Message:    app.MessageBadRequest,
			})
			return
		}
		placeIDs = append(placeIDs, placeID)
	}

	rates, err := h.repo.GetPlaceRates(c.Request.Context(), placeIDs)
	if err != nil {
		slog.Error("batch quote rate lookup failed", "count", len(placeIDs), "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[[]Quote]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}

	quotes := make([]Quote, 0, len(placeIDs))
	for _, placeID := range placeIDs {
		quotes = append(quotes, CalculateQuote(placeID, body.Hours, rates[placeID]))
	}

	wrapper.Respond(c, wrapper.ResponseOption[[]Quote]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       &quotes,
	})
}
