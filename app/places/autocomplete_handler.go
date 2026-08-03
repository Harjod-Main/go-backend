package places

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/RinTanth/go-common/app"
	"github.com/RinTanth/go-common/wrapper"
	"github.com/gin-gonic/gin"
)

const (
	maxAutocompleteQueryLen = 200
	maxSessionTokenLen      = 128
	maxPlaceIDLen           = 256
)

// Autocomplete handles GET /api/v1/places/autocomplete?q=&lat=&lng=&language=&sessionToken=
func (h *Handler) Autocomplete(c *gin.Context) {
	if h.google == nil {
		wrapper.Respond(c, wrapper.ResponseOption[AutocompleteResponse]{
			HTTPStatus: http.StatusServiceUnavailable,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}

	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		wrapper.Respond(c, wrapper.ResponseOption[AutocompleteResponse]{
			HTTPStatus: http.StatusOK,
			Code:       app.CodeSuccess,
			Message:    app.MessageSuccess,
			Data:       &AutocompleteResponse{Predictions: []PlacePrediction{}},
		})
		return
	}
	if len(q) > maxAutocompleteQueryLen {
		wrapper.Respond(c, wrapper.ResponseOption[AutocompleteResponse]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	lat, lng, ok := parseOptionalLatLng(c.Query("lat"), c.Query("lng"))
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[AutocompleteResponse]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	sessionToken := strings.TrimSpace(c.Query("sessionToken"))
	if len(sessionToken) > maxSessionTokenLen {
		wrapper.Respond(c, wrapper.ResponseOption[AutocompleteResponse]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	predictions, err := h.google.Autocomplete(c.Request.Context(), GoogleAutocompleteRequest{
		Input:        q,
		LanguageCode: c.Query("language"),
		SessionToken: sessionToken,
		Latitude:     lat,
		Longitude:    lng,
	})
	if err != nil {
		respondGooglePlacesAutocompleteError(c, err)
		return
	}

	resp := AutocompleteResponse{Predictions: predictions}
	wrapper.Respond(c, wrapper.ResponseOption[AutocompleteResponse]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       &resp,
	})
}

// GetPlaceDetails handles GET /api/v1/places/details/:placeId?language=&sessionToken=
func (h *Handler) GetPlaceDetails(c *gin.Context) {
	if h.google == nil {
		wrapper.Respond(c, wrapper.ResponseOption[PlaceDetails]{
			HTTPStatus: http.StatusServiceUnavailable,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}

	placeID := strings.TrimPrefix(strings.TrimSpace(c.Param("placeId")), "places/")
	if placeID == "" || len(placeID) > maxPlaceIDLen {
		wrapper.Respond(c, wrapper.ResponseOption[PlaceDetails]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	sessionToken := strings.TrimSpace(c.Query("sessionToken"))
	if len(sessionToken) > maxSessionTokenLen {
		wrapper.Respond(c, wrapper.ResponseOption[PlaceDetails]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	details, err := h.google.PlaceDetails(c.Request.Context(), GooglePlaceDetailsRequest{
		PlaceID:      placeID,
		LanguageCode: c.Query("language"),
		SessionToken: sessionToken,
	})
	if err != nil {
		respondGooglePlacesDetailsError(c, err)
		return
	}

	wrapper.Respond(c, wrapper.ResponseOption[PlaceDetails]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       details,
	})
}

func parseOptionalLatLng(latRaw, lngRaw string) (*float64, *float64, bool) {
	latRaw = strings.TrimSpace(latRaw)
	lngRaw = strings.TrimSpace(lngRaw)
	if latRaw == "" && lngRaw == "" {
		return nil, nil, true
	}
	if latRaw == "" || lngRaw == "" {
		return nil, nil, false
	}
	lat, errLat := strconv.ParseFloat(latRaw, 64)
	lng, errLng := strconv.ParseFloat(lngRaw, 64)
	if errLat != nil || errLng != nil {
		return nil, nil, false
	}
	if lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		return nil, nil, false
	}
	return &lat, &lng, true
}

func respondGooglePlacesAutocompleteError(c *gin.Context, err error) {
	if errors.Is(err, ErrGooglePlacesNotConfigured) {
		wrapper.Respond(c, wrapper.ResponseOption[AutocompleteResponse]{
			HTTPStatus: http.StatusServiceUnavailable,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}
	slog.Error("google places proxy failed", "op", "autocomplete", "error", err)
	wrapper.Respond(c, wrapper.ResponseOption[AutocompleteResponse]{
		HTTPStatus: http.StatusBadGateway,
		Code:       app.CodeInternalError,
		Message:    app.MessageInternalError,
	})
}

func respondGooglePlacesDetailsError(c *gin.Context, err error) {
	if errors.Is(err, ErrGooglePlacesNotConfigured) {
		wrapper.Respond(c, wrapper.ResponseOption[PlaceDetails]{
			HTTPStatus: http.StatusServiceUnavailable,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}
	slog.Error("google places proxy failed", "op", "details", "error", err)
	wrapper.Respond(c, wrapper.ResponseOption[PlaceDetails]{
		HTTPStatus: http.StatusBadGateway,
		Code:       app.CodeInternalError,
		Message:    app.MessageInternalError,
	})
}
