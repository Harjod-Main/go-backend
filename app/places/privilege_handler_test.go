package places_test

import (
	"bytes"
	"encoding/json"
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

func TestGetPrivileges_ReturnsPayload(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	payload := &places.PlacePrivileges{
		ValidationParking: []places.ValidationParking{{
			Validation: &places.Validation{
				ValidationID:         "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
				ValidationType:       "spending",
				ConditionDescription: "Spend 500",
			},
		}},
		ParkingArea: []places.PrivilegeArea{{
			Reserved: []places.Reserved{{
				ReservedID:      "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
				ReservationType: "membership",
			}},
			EVCharger: []places.EVCharger{},
		}},
	}

	engine := gin.New()
	handler := places.NewHandler(places.HandlerConfig{Repo: &stubRepo{privileges: payload}})
	engine.GET("/api/v1/places/:placeId/privileges", handler.GetPrivileges)

	placeID := "11111111-1111-1111-1111-111111111111"
	req := httptest.NewRequest(http.MethodGet, "/api/v1/places/"+placeID+"/privileges", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusOK, w.Code)

	var body struct {
		Data places.PlacePrivileges `json:"data"`
	}
	r.NoError(json.Unmarshal(w.Body.Bytes(), &body))
	r.Len(body.Data.ValidationParking, 1)
	r.Equal("spending", body.Data.ValidationParking[0].Validation.ValidationType)
	r.Len(body.Data.ParkingArea[0].Reserved, 1)
}

func TestGetPrivileges_EmptyWhenMissing(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	handler := places.NewHandler(places.HandlerConfig{Repo: &stubRepo{privileges: nil}})
	engine.GET("/api/v1/places/:placeId/privileges", handler.GetPrivileges)

	placeID := "11111111-1111-1111-1111-111111111111"
	req := httptest.NewRequest(http.MethodGet, "/api/v1/places/"+placeID+"/privileges", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusOK, w.Code)

	var body struct {
		Data places.PlacePrivileges `json:"data"`
	}
	r.NoError(json.Unmarshal(w.Body.Bytes(), &body))
	r.Empty(body.Data.ValidationParking)
	r.Empty(body.Data.ParkingArea)
}

func TestGetPrivilegeDetail_Stamp(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	validation := &places.Validation{
		ValidationID:   "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		ValidationType: "spending",
	}

	engine := gin.New()
	handler := places.NewHandler(places.HandlerConfig{Repo: &stubRepo{validation: validation}})
	engine.GET("/api/v1/privileges/:kind/:id", handler.GetPrivilegeDetail)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/privileges/stamp/"+validation.ValidationID, nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusOK, w.Code)

	var body struct {
		Data places.Validation `json:"data"`
	}
	r.NoError(json.Unmarshal(w.Body.Bytes(), &body))
	r.Equal("spending", body.Data.ValidationType)
}

func TestGetPrivilegeDetail_NotFound(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	handler := places.NewHandler(places.HandlerConfig{Repo: &stubRepo{}})
	engine.GET("/api/v1/privileges/:kind/:id", handler.GetPrivilegeDetail)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/privileges/stamp/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusNotFound, w.Code)

	var body struct {
		Code string `json:"code"`
	}
	r.NoError(json.Unmarshal(w.Body.Bytes(), &body))
	r.Equal(string(app.CodeNotFound), body.Code)
}

func TestGetPrivilegeDetail_InvalidKind(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	handler := places.NewHandler(places.HandlerConfig{Repo: &stubRepo{}})
	engine.GET("/api/v1/privileges/:kind/:id", handler.GetPrivilegeDetail)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/privileges/other/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusBadRequest, w.Code)
}

func TestUpdateStamp_Success(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	validationID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	location := "Information Counter (Zone A & D)"
	handler := places.NewHandler(places.HandlerConfig{
		Repo: &stubRepo{
			validation: &places.Validation{
				ValidationID:   validationID,
				ValidationType: "other",
			},
			firstCorrection: true,
		},
		Profiles: &stubProfiles{total: 10},
	})
	engine := gin.New()
	engine.PATCH("/api/v1/privileges/stamp/:id", func(c *gin.Context) {
		c.Set(supabaseauth.CtxClaimsKey, &supabaseauth.Claims{
			Sub: "11111111-1111-1111-1111-111111111111",
		})
		handler.UpdateStamp(c)
	})

	payload, err := json.Marshal(map[string]any{
		"category":              "OTHER",
		"condition_description": "ฟรี 3 ชั่วโมงแรก",
		"notes":                 "ใบเสร็จงานอีเวนต์ใช้ไม่ได้",
		"location":              location,
	})
	r.NoError(err)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/privileges/stamp/"+validationID, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusOK, w.Code)

	var body struct {
		Data places.StampCorrectionResult `json:"data"`
	}
	r.NoError(json.Unmarshal(w.Body.Bytes(), &body))
	r.Equal("other", body.Data.Validation.ValidationType)
	r.Equal("ฟรี 3 ชั่วโมงแรก", body.Data.Validation.ConditionDescription)
	r.Equal(10, body.Data.PointsAwarded)
}

func TestUpdateStamp_Unauthorized(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	handler := places.NewHandler(places.HandlerConfig{Repo: &stubRepo{}})
	engine := gin.New()
	engine.PATCH("/api/v1/privileges/stamp/:id", handler.UpdateStamp)

	req := httptest.NewRequest(
		http.MethodPatch,
		"/api/v1/privileges/stamp/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		strings.NewReader(`{"category":"OTHER","condition_description":"test"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusUnauthorized, w.Code)
}
