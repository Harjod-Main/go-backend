package places

import (
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/RinTanth/go-common/app"
	"github.com/RinTanth/go-common/wrapper"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type quoteRequestBody struct {
	Hours    float64  `json:"hours"`
	PlaceIDs []string `json:"placeIds"`
}

// maxQuoteHours caps stay duration on public quote endpoints (30 days).
const maxQuoteHours = 720
const maxCreateQuotesBodyBytes = 16 * 1024

var quoteLocation = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		return time.FixedZone("ICT", 7*3600)
	}
	return loc
}()

func quoteNow() time.Time {
	return time.Now().In(quoteLocation)
}

func (h *Handler) stampMinutes(c *gin.Context, placeIDs []string) (map[string]int, error) {
	stamps, err := h.repo.WalkInStampFreeMinutes(c.Request.Context(), placeIDs)
	if err != nil {
		return nil, err
	}
	if stamps == nil {
		return map[string]int{}, nil
	}
	return stamps, nil
}

func isValidQuoteHours(hours float64) bool {
	return hours >= 0 && hours <= maxQuoteHours && !math.IsNaN(hours) && !math.IsInf(hours, 0)
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
	if err != nil || !isValidQuoteHours(hours) {
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

	stamps, err := h.stampMinutes(c, []string{placeID})
	if err != nil {
		slog.Error("quote stamp lookup failed", "place_id", placeID, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[Quote]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}

	quote := CalculateQuoteOpts(placeID, hours, rate, QuoteOptions{
		StampFreeMinutes: stamps[placeID],
		Now:              quoteNow(),
	})
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
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxCreateQuotesBodyBytes)

	var body quoteRequestBody
	if err := c.ShouldBindJSON(&body); err != nil || !isValidQuoteHours(body.Hours) || len(body.PlaceIDs) == 0 {
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

	stamps, err := h.stampMinutes(c, placeIDs)
	if err != nil {
		slog.Error("batch quote stamp lookup failed", "count", len(placeIDs), "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[[]Quote]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}

	now := quoteNow()
	quotes := make([]Quote, 0, len(placeIDs))
	for _, placeID := range placeIDs {
		quotes = append(quotes, CalculateQuoteOpts(placeID, body.Hours, rates[placeID], QuoteOptions{
			StampFreeMinutes: stamps[placeID],
			Now:              now,
		}))
	}

	wrapper.Respond(c, wrapper.ResponseOption[[]Quote]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       &quotes,
	})
}
