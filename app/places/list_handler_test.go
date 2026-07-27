package places_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RinTanth/go-backend/app/places"
	"github.com/RinTanth/go-common/app"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type stubRepo struct {
	places  []places.Place
	err     error
	rate    *places.PlaceRateDetail
	rateErr error
}

func (s stubRepo) ListMapPlaces(context.Context) ([]places.Place, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.places, nil
}

func (s stubRepo) GetPlaceRate(context.Context, string) (*places.PlaceRateDetail, error) {
	if s.rateErr != nil {
		return nil, s.rateErr
	}
	return s.rate, nil
}

func TestList_ReturnsPlaces(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	freeMinutes := 30
	sample := []places.Place{{
		PlaceID:   "11111111-1111-1111-1111-111111111111",
		NameTh:    "สยามพารากอน",
		NameEn:    "Siam Paragon",
		PlaceType: "shopping_mall",
		Latitude:  13.746,
		Longitude: 100.535,
		ParkingArea: []places.ParkingArea{{
			Rate: []places.Rate{{
				FreeMinutes: &freeMinutes,
				RateTier: []places.RateTier{{
					TierOrder: 1,
					Price:     40,
					Unit:      "hourly",
				}},
			}},
		}},
	}}

	engine := gin.New()
	handler := places.NewHandler(places.HandlerConfig{Repo: stubRepo{places: sample}})
	engine.GET("/api/v1/places", handler.List)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/places", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusOK, w.Code)

	var body struct {
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Data    []places.Place `json:"data"`
	}
	r.NoError(json.Unmarshal(w.Body.Bytes(), &body))
	r.Equal(string(app.CodeSuccess), body.Code)
	r.Equal(string(app.MessageSuccess), body.Message)
	r.Len(body.Data, 1)
	r.Equal("Siam Paragon", body.Data[0].NameEn)
	r.Equal("shopping_mall", body.Data[0].PlaceType)
	r.Equal(30, *body.Data[0].ParkingArea[0].Rate[0].FreeMinutes)
	r.Equal(40.0, body.Data[0].ParkingArea[0].Rate[0].RateTier[0].Price)
}

func TestList_RepoError(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	handler := places.NewHandler(places.HandlerConfig{
		Repo: stubRepo{err: errors.New("db down")},
	})
	engine.GET("/api/v1/places", handler.List)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/places", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusInternalServerError, w.Code)

	var body struct {
		Code string `json:"code"`
	}
	r.NoError(json.Unmarshal(w.Body.Bytes(), &body))
	r.Equal(string(app.CodeInternalError), body.Code)
}

func floatPtr(v float64) *float64 { return &v }

func strPtr(s string) *string { return &s }
