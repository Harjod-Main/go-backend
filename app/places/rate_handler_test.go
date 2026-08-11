package places_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/RinTanth/go-backend/app/auth/supabaseauth"
	"github.com/RinTanth/go-backend/app/places"
	"github.com/RinTanth/go-common/app"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGetRate_ReturnsRate(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	freeMinutes := 30
	detail := &places.PlaceRateDetail{
		FreeMinutes: &freeMinutes,
		Currency:    strPtr("THB"),
		RateTier: []places.PlaceRateTier{{
			TierOrder: 1,
			FromHour:  0,
			ToHour:    floatPtr(1),
			Price:     40,
			Unit:      "hourly",
		}},
	}

	engine := gin.New()
	handler := places.NewHandler(places.HandlerConfig{Repo: &stubRepo{rate: detail}})
	engine.GET("/api/v1/places/:placeId/rate", handler.GetRate)

	placeID := "11111111-1111-1111-1111-111111111111"
	req := httptest.NewRequest(http.MethodGet, "/api/v1/places/"+placeID+"/rate", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusOK, w.Code)

	var body struct {
		Data *places.PlaceRateDetail `json:"data"`
	}
	r.NoError(json.Unmarshal(w.Body.Bytes(), &body))
	r.NotNil(body.Data)
	r.Equal(30, *body.Data.FreeMinutes)
	r.Len(body.Data.RateTier, 1)
	r.Equal(40.0, body.Data.RateTier[0].Price)
}

func TestGetRate_NotFound(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	handler := places.NewHandler(places.HandlerConfig{Repo: &stubRepo{rate: nil}})
	engine.GET("/api/v1/places/:placeId/rate", handler.GetRate)

	placeID := "22222222-2222-2222-2222-222222222222"
	req := httptest.NewRequest(http.MethodGet, "/api/v1/places/"+placeID+"/rate", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusOK, w.Code)

	var body struct {
		Data *places.PlaceRateDetail `json:"data"`
	}
	r.NoError(json.Unmarshal(w.Body.Bytes(), &body))
	r.Nil(body.Data)
}

func TestGetRate_InvalidPlaceID(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	handler := places.NewHandler(places.HandlerConfig{Repo: &stubRepo{}})
	engine.GET("/api/v1/places/:placeId/rate", handler.GetRate)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/places/not-a-uuid/rate", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusBadRequest, w.Code)

	var body struct {
		Code string `json:"code"`
	}
	r.NoError(json.Unmarshal(w.Body.Bytes(), &body))
	r.Equal(string(app.CodeBadRequest), body.Code)
}

func TestGetRate_RepoError(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	handler := places.NewHandler(places.HandlerConfig{
		Repo: &stubRepo{rateErr: errors.New("db down")},
	})
	engine.GET("/api/v1/places/:placeId/rate", handler.GetRate)

	placeID := "11111111-1111-1111-1111-111111111111"
	req := httptest.NewRequest(http.MethodGet, "/api/v1/places/"+placeID+"/rate", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusInternalServerError, w.Code)
}

func TestUpdateRate_Success(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	placeID := "11111111-1111-1111-1111-111111111111"
	freeMinutes := 60
	updated := &places.PlaceRateDetail{
		FreeMinutes: &freeMinutes,
		Currency:    strPtr("THB"),
		RateTier: []places.PlaceRateTier{{
			TierOrder: 1,
			FromHour:  0,
			ToHour:    floatPtr(1),
			Price:     50,
			Unit:      "hourly",
		}},
	}

	handler := places.NewHandler(places.HandlerConfig{
		Repo: &stubRepo{
			rate:            &places.PlaceRateDetail{FreeMinutes: &freeMinutes},
			updatedRate:    updated,
			firstCorrection: true,
		},
		Profiles: &stubProfiles{total: 10},
	})

	engine := gin.New()
	engine.PATCH("/api/v1/places/:placeId/rate", func(c *gin.Context) {
		c.Set(supabaseauth.CtxClaimsKey, &supabaseauth.Claims{
			Sub: "22222222-2222-2222-2222-222222222222",
		})
		handler.UpdateRate(c)
	})

	payload, err := json.Marshal(map[string]any{
		"freeMinutes": 30,
		"lostTicketFee": nil,
		"overnightFee":  nil,
		"specialConditions": []string{
			"Rate resets at midnight",
		},
		"rateTiers": []map[string]any{
			{
				"fromHour": 0,
				"toHour":   1,
				"pricePerHour": 50,
				"unit":      "hourly",
			},
		},
	})
	r.NoError(err)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/places/"+placeID+"/rate", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusOK, w.Code)

	var body struct {
		Data places.RateCorrectionResult `json:"data"`
	}
	r.NoError(json.Unmarshal(w.Body.Bytes(), &body))
	r.NotNil(body.Data.Rate.FreeMinutes)
	r.Equal(60, *body.Data.Rate.FreeMinutes)
	r.Equal(10, body.Data.PointsAwarded)
}

func TestUpdateRate_Unauthorized(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	handler := places.NewHandler(places.HandlerConfig{Repo: &stubRepo{}})

	engine := gin.New()
	engine.PATCH("/api/v1/places/:placeId/rate", handler.UpdateRate)

	req := httptest.NewRequest(
		http.MethodPatch,
		"/api/v1/places/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/rate",
		strings.NewReader(`{"freeMinutes":30,"rateTiers":[{"fromHour":0,"toHour":1,"pricePerHour":50,"unit":"hourly"}]}`),
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusUnauthorized, w.Code)
}
