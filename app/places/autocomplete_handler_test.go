package places_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RinTanth/go-backend/app/places"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type stubGoogle struct {
	predictions []places.PlacePrediction
	details     *places.PlaceDetails
	autoErr     error
	detailsErr  error
	lastAuto    places.GoogleAutocompleteRequest
	lastDetails places.GooglePlaceDetailsRequest
}

func (s *stubGoogle) Autocomplete(_ context.Context, req places.GoogleAutocompleteRequest) ([]places.PlacePrediction, error) {
	s.lastAuto = req
	if s.autoErr != nil {
		return nil, s.autoErr
	}
	return s.predictions, nil
}

func (s *stubGoogle) PlaceDetails(_ context.Context, req places.GooglePlaceDetailsRequest) (*places.PlaceDetails, error) {
	s.lastDetails = req
	if s.detailsErr != nil {
		return nil, s.detailsErr
	}
	return s.details, nil
}

func TestAutocomplete_Success(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	google := &stubGoogle{
		predictions: []places.PlacePrediction{{
			PlaceID: "ChIJ_test",
			Name:    "Siam Paragon",
			Address: "Bangkok",
		}},
	}
	handler := places.NewHandler(places.HandlerConfig{
		Repo:   &stubRepo{},
		Google: google,
	})
	engine := gin.New()
	engine.GET("/api/v1/places/autocomplete", handler.Autocomplete)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/places/autocomplete?q=siam&lat=13.7&lng=100.5&language=th&sessionToken=tok-1",
		nil,
	)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusOK, w.Code)
	r.Equal("siam", google.lastAuto.Input)
	r.Equal("th", google.lastAuto.LanguageCode)
	r.Equal("tok-1", google.lastAuto.SessionToken)
	r.NotNil(google.lastAuto.Latitude)
	r.InDelta(13.7, *google.lastAuto.Latitude, 0.0001)

	var body struct {
		Data places.AutocompleteResponse `json:"data"`
	}
	r.NoError(json.Unmarshal(w.Body.Bytes(), &body))
	r.Len(body.Data.Predictions, 1)
	r.Equal("Siam Paragon", body.Data.Predictions[0].Name)
}

func TestAutocomplete_EmptyQueryReturnsEmptyList(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	handler := places.NewHandler(places.HandlerConfig{
		Repo:   &stubRepo{},
		Google: &stubGoogle{},
	})
	engine := gin.New()
	engine.GET("/api/v1/places/autocomplete", handler.Autocomplete)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/places/autocomplete?q=", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusOK, w.Code)
}

func TestAutocomplete_RejectsInvalidLatLng(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	handler := places.NewHandler(places.HandlerConfig{
		Repo:   &stubRepo{},
		Google: &stubGoogle{},
	})
	engine := gin.New()
	engine.GET("/api/v1/places/autocomplete", handler.Autocomplete)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/places/autocomplete?q=siam&lat=13.7", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusBadRequest, w.Code)
}

func TestAutocomplete_NotConfigured(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	handler := places.NewHandler(places.HandlerConfig{
		Repo:   &stubRepo{},
		Google: places.NewGooglePlacesClient(""),
	})
	engine := gin.New()
	engine.GET("/api/v1/places/autocomplete", handler.Autocomplete)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/places/autocomplete?q=siam", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusServiceUnavailable, w.Code)
}

func TestGetPlaceDetails_Success(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	google := &stubGoogle{
		details: &places.PlaceDetails{
			PlaceID:   "ChIJ_test",
			Name:      "Siam Paragon",
			Address:   "Bangkok",
			Latitude:  13.746,
			Longitude: 100.535,
		},
	}
	handler := places.NewHandler(places.HandlerConfig{
		Repo:   &stubRepo{},
		Google: google,
	})
	engine := gin.New()
	engine.GET("/api/v1/places/details/:placeId", handler.GetPlaceDetails)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/places/details/ChIJ_test?language=en&sessionToken=tok-2",
		nil,
	)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusOK, w.Code)
	r.Equal("ChIJ_test", google.lastDetails.PlaceID)
	r.Equal("tok-2", google.lastDetails.SessionToken)

	var body struct {
		Data places.PlaceDetails `json:"data"`
	}
	r.NoError(json.Unmarshal(w.Body.Bytes(), &body))
	r.Equal("Siam Paragon", body.Data.Name)
	r.InDelta(13.746, body.Data.Latitude, 0.0001)
}
