package places_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RinTanth/go-backend/app/places"
	"github.com/RinTanth/go-common/app"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCreateQuotes_BatchLoadsRatesOnce(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	id1 := "11111111-1111-1111-1111-111111111111"
	id2 := "22222222-2222-2222-2222-222222222222"
	rate := &places.PlaceRateDetail{
		FreeMinutes: intPtr(0),
		Currency:    sPtr("THB"),
		RateTier: []places.PlaceRateTier{{
			TierOrder: 1, FromHour: 0, ToHour: f64Ptr(24), Price: 40, Unit: "hourly",
		}},
	}
	repo := &stubRepo{rates: map[string]*places.PlaceRateDetail{
		id1: rate,
		id2: rate,
	}}

	engine := gin.New()
	handler := places.NewHandler(places.HandlerConfig{Repo: repo})
	engine.POST("/api/v1/quotes", handler.CreateQuotes)

	payload, err := json.Marshal(map[string]any{
		"hours":    2,
		"placeIds": []string{id1, id2},
	})
	r.NoError(err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/quotes", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusOK, w.Code)
	r.Equal(int32(1), repo.ratesCalls.Load(), "expected a single batch GetPlaceRates call")

	var body struct {
		Code string         `json:"code"`
		Data []places.Quote `json:"data"`
	}
	r.NoError(json.Unmarshal(w.Body.Bytes(), &body))
	r.Equal(string(app.CodeSuccess), body.Code)
	r.Len(body.Data, 2)
	r.Equal(id1, body.Data[0].PlaceID)
	r.Equal(id2, body.Data[1].PlaceID)
	r.Equal(80.0, body.Data[0].Total)
	r.Equal(80.0, body.Data[1].Total)
}

func TestCreateQuotes_InvalidPlaceID(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	repo := &stubRepo{}
	engine := gin.New()
	handler := places.NewHandler(places.HandlerConfig{Repo: repo})
	engine.POST("/api/v1/quotes", handler.CreateQuotes)

	payload, err := json.Marshal(map[string]any{
		"hours":    1,
		"placeIds": []string{"not-a-uuid"},
	})
	r.NoError(err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/quotes", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusBadRequest, w.Code)
	r.Equal(int32(0), repo.ratesCalls.Load())
}

func TestGetQuote_RejectsExcessiveHours(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	placeID := "11111111-1111-1111-1111-111111111111"
	repo := &stubRepo{rate: &places.PlaceRateDetail{
		FreeMinutes: intPtr(0),
		Currency:    sPtr("THB"),
		RateTier: []places.PlaceRateTier{{
			TierOrder: 1, FromHour: 0, ToHour: f64Ptr(24), Price: 40, Unit: "hourly",
		}},
	}}

	engine := gin.New()
	handler := places.NewHandler(places.HandlerConfig{Repo: repo})
	engine.GET("/api/v1/places/:placeId/quote", handler.GetQuote)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/places/"+placeID+"/quote?hours=1e9", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusBadRequest, w.Code)
}

func TestCreateQuotes_RejectsExcessiveHours(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	repo := &stubRepo{}
	engine := gin.New()
	handler := places.NewHandler(places.HandlerConfig{Repo: repo})
	engine.POST("/api/v1/quotes", handler.CreateQuotes)

	payload, err := json.Marshal(map[string]any{
		"hours":    1e9,
		"placeIds": []string{"11111111-1111-1111-1111-111111111111"},
	})
	r.NoError(err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/quotes", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusBadRequest, w.Code)
	r.Equal(int32(0), repo.ratesCalls.Load())
}
