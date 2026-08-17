package places_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/RinTanth/go-backend/app/auth/supabaseauth"
	"github.com/RinTanth/go-backend/app/mediaurl"
	"github.com/RinTanth/go-backend/app/places"
	"github.com/RinTanth/go-common/app"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const testMediaPrefix = "https://sycwdwymeirxowbrqdgd.supabase.co/storage/v1/object/public/media/"

func TestMain(m *testing.M) {
	mediaurl.Configure("https://sycwdwymeirxowbrqdgd.supabase.co")
	code := m.Run()
	mediaurl.ResetForTest()
	os.Exit(code)
}

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
		SignagePhotos:  []string{testMediaPrefix + "11111111-1111-1111-1111-111111111111/submissions/a.jpg"},
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
	r.Equal([]string{testMediaPrefix + "11111111-1111-1111-1111-111111111111/submissions/a.jpg"}, body.Data.SignagePhotos)
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

func TestUpdateReserved_Success(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	reservedID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	location := "1st Floor"
	rule := "2 hrs limit then will charge"
	handler := places.NewHandler(places.HandlerConfig{
		Repo: &stubRepo{
			reserved: &places.Reserved{
				ReservedID:      reservedID,
				ReservationType: "cardholder",
			},
			firstCorrection: true,
		},
		Profiles: &stubProfiles{total: 10},
	})
	engine := gin.New()
	engine.PATCH("/api/v1/privileges/reserve/:id", func(c *gin.Context) {
		c.Set(supabaseauth.CtxClaimsKey, &supabaseauth.Claims{
			Sub: "11111111-1111-1111-1111-111111111111",
		})
		handler.UpdateReserved(c)
	})

	payload, err := json.Marshal(map[string]any{
		"category": "CREDITCARD_HOLDERS",
		"name":     "SCB M Visa Card",
		"rule":     rule,
		"location": location,
	})
	r.NoError(err)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/privileges/reserve/"+reservedID, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusOK, w.Code)

	var body struct {
		Data places.ReservedCorrectionResult `json:"data"`
	}
	r.NoError(json.Unmarshal(w.Body.Bytes(), &body))
	r.Equal("cardholder", body.Data.Reserved.ReservationType)
	r.Equal("SCB M Visa Card", *body.Data.Reserved.ProgramOther)
	r.Equal(rule, *body.Data.Reserved.Conditions)
	r.Equal(location, *body.Data.Reserved.Floor)
	r.Equal(10, body.Data.PointsAwarded)
}

func TestUpdateStamp_PersistsSignagePhotos(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	validationID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	photo := testMediaPrefix + "11111111-1111-1111-1111-111111111111/submissions/sign.jpg"
	repo := &stubRepo{
		validation: &places.Validation{
			ValidationID:   validationID,
			ValidationType: "other",
		},
	}
	handler := places.NewHandler(places.HandlerConfig{Repo: repo, Profiles: &stubProfiles{total: 10}})
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
		"signagePhotos":         []string{photo},
	})
	r.NoError(err)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/privileges/stamp/"+validationID, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusOK, w.Code)
	r.NotNil(repo.lastStampUpdate)
	r.NotNil(repo.lastStampUpdate.SignagePhotos)
	r.Equal([]string{photo}, *repo.lastStampUpdate.SignagePhotos)

	var body struct {
		Data places.StampCorrectionResult `json:"data"`
	}
	r.NoError(json.Unmarshal(w.Body.Bytes(), &body))
	r.Equal([]string{photo}, body.Data.Validation.SignagePhotos)
}

func TestUpdateStamp_RejectsInvalidSignagePhotos(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	handler := places.NewHandler(places.HandlerConfig{
		Repo: &stubRepo{
			validation: &places.Validation{
				ValidationID:   "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
				ValidationType: "other",
			},
		},
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
		"condition_description": "test",
		"signagePhotos":         []string{"https://evil.example.com/not-allowed.jpg"},
	})
	r.NoError(err)

	req := httptest.NewRequest(
		http.MethodPatch,
		"/api/v1/privileges/stamp/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		bytes.NewReader(payload),
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusBadRequest, w.Code)
}

func TestUpdateReserved_PersistsSignagePhotos(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	reservedID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	photo := testMediaPrefix + "11111111-1111-1111-1111-111111111111/submissions/reserve.jpg"
	repo := &stubRepo{
		reserved: &places.Reserved{
			ReservedID:      reservedID,
			ReservationType: "cardholder",
		},
	}
	handler := places.NewHandler(places.HandlerConfig{Repo: repo, Profiles: &stubProfiles{total: 10}})
	engine := gin.New()
	engine.PATCH("/api/v1/privileges/reserve/:id", func(c *gin.Context) {
		c.Set(supabaseauth.CtxClaimsKey, &supabaseauth.Claims{
			Sub: "11111111-1111-1111-1111-111111111111",
		})
		handler.UpdateReserved(c)
	})

	payload, err := json.Marshal(map[string]any{
		"category":      "CREDITCARD_HOLDERS",
		"name":          "SCB M Visa Card",
		"signagePhotos": []string{photo},
	})
	r.NoError(err)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/privileges/reserve/"+reservedID, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusOK, w.Code)
	r.NotNil(repo.lastReserveUpdate)
	r.NotNil(repo.lastReserveUpdate.SignagePhotos)
	r.Equal([]string{photo}, *repo.lastReserveUpdate.SignagePhotos)
}

func TestGetPrivilegeDetail_EV(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	rule := "Members only"
	floor := "B2"
	charger := &places.EVCharger{
		EVChargerID: "cccccccc-cccc-cccc-cccc-cccccccccccc",
		PlaceID:     "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		Floor:       &floor,
		Conditions:  &rule,
		EVProvider:  &places.EVProvider{Name: "EA Anywhere"},
		EVConnector: []places.EVConnector{{ConnectorType: "AC_Type_2"}},
		SignagePhotos: []string{
			testMediaPrefix + "11111111-1111-1111-1111-111111111111/submissions/ev.jpg",
		},
	}

	engine := gin.New()
	handler := places.NewHandler(places.HandlerConfig{Repo: &stubRepo{evCharger: charger}})
	engine.GET("/api/v1/privileges/:kind/:id", handler.GetPrivilegeDetail)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/privileges/ev/"+charger.EVChargerID, nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusOK, w.Code)
	var body struct {
		Data places.EVCharger `json:"data"`
	}
	r.NoError(json.Unmarshal(w.Body.Bytes(), &body))
	r.Equal("EA Anywhere", body.Data.EVProvider.Name)
	r.Equal(charger.PlaceID, body.Data.PlaceID)
	r.Equal(rule, *body.Data.Conditions)
	r.Equal(charger.SignagePhotos, body.Data.SignagePhotos)
}

func TestUpdateEV_Success(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	chargerID := "cccccccc-cccc-cccc-cccc-cccccccccccc"
	handler := places.NewHandler(places.HandlerConfig{
		Repo: &stubRepo{
			evCharger: &places.EVCharger{
				EVChargerID: chargerID,
				EVProvider:  &places.EVProvider{Name: "Tesla Supercharger"},
			},
			firstCorrection: true,
		},
		Profiles: &stubProfiles{total: 10},
	})
	engine := gin.New()
	engine.PATCH("/api/v1/privileges/ev/:id", func(c *gin.Context) {
		c.Set(supabaseauth.CtxClaimsKey, &supabaseauth.Claims{
			Sub: "11111111-1111-1111-1111-111111111111",
		})
		handler.UpdateEV(c)
	})

	payload, err := json.Marshal(map[string]any{
		"providerName": "ea_anywhere",
		"connectors":   []map[string]string{{"connectorType": "TYPE_2", "total": "2"}},
		"rule":         "Free for members",
		"location":     "B2",
	})
	r.NoError(err)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/privileges/ev/"+chargerID, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusOK, w.Code)
	var body struct {
		Data places.EVCorrectionResult `json:"data"`
	}
	r.NoError(json.Unmarshal(w.Body.Bytes(), &body))
	r.Equal("EA Anywhere", body.Data.EVCharger.EVProvider.Name)
	r.Equal("B2", *body.Data.EVCharger.Floor)
	r.Equal("Free for members", *body.Data.EVCharger.Conditions)
	r.Len(body.Data.EVCharger.EVConnector, 2)
	r.Equal(10, body.Data.PointsAwarded)
}

func TestUpdateEV_PersistsSignagePhotos(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	chargerID := "cccccccc-cccc-cccc-cccc-cccccccccccc"
	photo := testMediaPrefix + "11111111-1111-1111-1111-111111111111/submissions/ev.jpg"
	repo := &stubRepo{
		evCharger: &places.EVCharger{EVChargerID: chargerID},
	}
	handler := places.NewHandler(places.HandlerConfig{Repo: repo, Profiles: &stubProfiles{total: 10}})
	engine := gin.New()
	engine.PATCH("/api/v1/privileges/ev/:id", func(c *gin.Context) {
		c.Set(supabaseauth.CtxClaimsKey, &supabaseauth.Claims{
			Sub: "11111111-1111-1111-1111-111111111111",
		})
		handler.UpdateEV(c)
	})

	payload, err := json.Marshal(map[string]any{
		"providerName":  "tesla",
		"connectors":    []map[string]string{{"connectorType": "TESLA", "total": "1"}},
		"signagePhotos": []string{photo},
	})
	r.NoError(err)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/privileges/ev/"+chargerID, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusOK, w.Code)
	r.NotNil(repo.lastEVUpdate)
	r.NotNil(repo.lastEVUpdate.SignagePhotos)
	r.Equal([]string{photo}, *repo.lastEVUpdate.SignagePhotos)
}

func TestUpdateEV_RejectsInvalidSignagePhotos(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	handler := places.NewHandler(places.HandlerConfig{
		Repo: &stubRepo{
			evCharger: &places.EVCharger{EVChargerID: "cccccccc-cccc-cccc-cccc-cccccccccccc"},
		},
	})
	engine := gin.New()
	engine.PATCH("/api/v1/privileges/ev/:id", func(c *gin.Context) {
		c.Set(supabaseauth.CtxClaimsKey, &supabaseauth.Claims{
			Sub: "11111111-1111-1111-1111-111111111111",
		})
		handler.UpdateEV(c)
	})

	payload, err := json.Marshal(map[string]any{
		"providerName":  "ea_anywhere",
		"connectors":    []map[string]string{{"connectorType": "TYPE_2", "total": "1"}},
		"signagePhotos": []string{"https://evil.example.com/not-allowed.jpg"},
	})
	r.NoError(err)

	req := httptest.NewRequest(
		http.MethodPatch,
		"/api/v1/privileges/ev/cccccccc-cccc-cccc-cccc-cccccccccccc",
		bytes.NewReader(payload),
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusBadRequest, w.Code)
}


