package places_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/RinTanth/go-backend/app/places"
	"github.com/RinTanth/go-backend/app/profile"
	"github.com/RinTanth/go-common/app"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type stubRepo struct {
	places            []places.Place
	err               error
	listCalls         atomic.Int32
	rate              *places.PlaceRateDetail
	updatedRate       *places.PlaceRateDetail
	amenities         *places.ParkingAmenitiesCorrectionResult
	rates             map[string]*places.PlaceRateDetail
	rateErr           error
	ratesCalls        atomic.Int32
	privileges        *places.PlacePrivileges
	privilegesErr     error
	validation        *places.Validation
	reserved          *places.Reserved
	evCharger         *places.EVCharger
	detailErr         error
	updatedValidation *places.Validation
	firstCorrection   bool
	updateErr         error
	lastStampUpdate   *places.UpdateValidationInput
	lastReserveUpdate *places.UpdateReservedInput
}

func (s *stubRepo) ListMapPlaces(context.Context) ([]places.Place, error) {
	s.listCalls.Add(1)
	if s.err != nil {
		return nil, s.err
	}
	return s.places, nil
}

func (s *stubRepo) GetPlaceRate(_ context.Context, placeID string) (*places.PlaceRateDetail, error) {
	if s.rateErr != nil {
		return nil, s.rateErr
	}
	if s.rates != nil {
		return s.rates[placeID], nil
	}
	return s.rate, nil
}

func (s *stubRepo) UpdateRate(_ context.Context, _ string, _ places.UpdateRateInput) (*places.PlaceRateDetail, bool, error) {
	if s.updateErr != nil {
		return nil, false, s.updateErr
	}
	if s.rate == nil {
		return nil, false, nil
	}
	updated := s.rate
	if s.updatedRate != nil {
		updated = s.updatedRate
	}
	return updated, s.firstCorrection, nil
}

func (s *stubRepo) UpdateParkingAmenities(
	_ context.Context,
	_ string,
	_ places.UpdateParkingAmenitiesInput,
) (*places.ParkingAmenitiesCorrectionResult, bool, error) {
	if s.detailErr != nil {
		return nil, false, s.detailErr
	}
	if s.amenities == nil {
		return nil, false, nil
	}
	return s.amenities, s.firstCorrection, nil
}

func (s *stubRepo) GetPlaceRates(_ context.Context, placeIDs []string) (map[string]*places.PlaceRateDetail, error) {
	s.ratesCalls.Add(1)
	if s.rateErr != nil {
		return nil, s.rateErr
	}
	out := make(map[string]*places.PlaceRateDetail, len(placeIDs))
	if s.rates != nil {
		for _, id := range placeIDs {
			if rate, ok := s.rates[id]; ok {
				out[id] = rate
			}
		}
		return out, nil
	}
	if s.rate != nil {
		for _, id := range placeIDs {
			out[id] = s.rate
		}
	}
	return out, nil
}

func (s *stubRepo) GetPlacePrivileges(context.Context, string) (*places.PlacePrivileges, error) {
	if s.privilegesErr != nil {
		return nil, s.privilegesErr
	}
	return s.privileges, nil
}

func (s *stubRepo) GetValidation(context.Context, string) (*places.Validation, error) {
	if s.detailErr != nil {
		return nil, s.detailErr
	}
	return s.validation, nil
}

func (s *stubRepo) UpdateValidation(
	_ context.Context,
	_ string,
	in places.UpdateValidationInput,
) (*places.Validation, bool, error) {
	copied := in
	s.lastStampUpdate = &copied
	if s.updateErr != nil {
		return nil, false, s.updateErr
	}
	if s.updatedValidation != nil {
		return s.updatedValidation, s.firstCorrection, nil
	}
	if s.validation == nil {
		return nil, false, nil
	}
	updated := *s.validation
	updated.ValidationType = in.ValidationType
	updated.ConditionDescription = in.ConditionDescription
	updated.Notes = in.Notes
	updated.ValidationLocation = in.ValidationLocation
	if in.SignagePhotos != nil {
		updated.SignagePhotos = append([]string{}, *in.SignagePhotos...)
	}
	return &updated, s.firstCorrection, nil
}

func (s *stubRepo) GetReserved(context.Context, string) (*places.Reserved, error) {
	if s.detailErr != nil {
		return nil, s.detailErr
	}
	return s.reserved, nil
}

func (s *stubRepo) UpdateReserved(
	_ context.Context,
	_ string,
	in places.UpdateReservedInput,
) (*places.Reserved, bool, error) {
	copied := in
	s.lastReserveUpdate = &copied
	if s.updateErr != nil {
		return nil, false, s.updateErr
	}
	if s.reserved == nil {
		return nil, false, nil
	}
	updated := *s.reserved
	updated.ReservationType = in.ReservationType
	updated.ProgramOther = in.ProgramOther
	updated.Conditions = in.Conditions
	updated.Floor = in.Floor
	updated.Program = nil
	if in.SignagePhotos != nil {
		updated.SignagePhotos = append([]string{}, *in.SignagePhotos...)
	}
	return &updated, s.firstCorrection, nil
}

func (s *stubRepo) GetEVCharger(context.Context, string) (*places.EVCharger, error) {
	if s.detailErr != nil {
		return nil, s.detailErr
	}
	return s.evCharger, nil
}

func (s *stubRepo) GetParkingAreaForPlace(context.Context, string) (*places.ParkingAreaRef, error) {
	return &places.ParkingAreaRef{
		ParkingAreaID: "cccccccc-cccc-cccc-cccc-cccccccccccc",
		Latitude:      13.7,
		Longitude:     100.5,
	}, nil
}

func (s *stubRepo) CreatePrivilege(context.Context, places.CreatePrivilegeInput) error {
	return nil
}

func (s *stubRepo) PlaceExists(context.Context, string) (bool, error) {
	return true, nil
}

func (s *stubRepo) GetPlaceReaction(context.Context, string, string) (*places.PlaceReactionResponse, error) {
	return &places.PlaceReactionResponse{}, nil
}

func (s *stubRepo) SetPlaceReaction(
	context.Context,
	string,
	string,
	places.PlaceReactionKind,
) (*places.PlaceReactionResponse, error) {
	like := places.PlaceReactionLike
	return &places.PlaceReactionResponse{MyReaction: &like}, nil
}

func (s *stubRepo) ClearPlaceReaction(context.Context, string, string) (*places.PlaceReactionResponse, error) {
	return &places.PlaceReactionResponse{}, nil
}

type stubProfiles struct {
	total int
}

func (s *stubProfiles) GetByUserID(context.Context, string) (*profile.Profile, error) {
	return nil, profile.ErrNotFound
}
func (s *stubProfiles) Ensure(_ context.Context, userID, _ string, _ profile.OAuthSeed) (*profile.Profile, error) {
	return &profile.Profile{UserID: userID, DisplayName: "u", Username: "user"}, nil
}
func (s *stubProfiles) SyncFromOAuth(context.Context, string, string, profile.OAuthSeed) (*profile.Profile, error) {
	return nil, profile.ErrNotFound
}
func (s *stubProfiles) Update(context.Context, string, *string, *string, *string, bool) (*profile.Profile, error) {
	return nil, profile.ErrNotFound
}
func (s *stubProfiles) AddCreditPoints(_ context.Context, _ string, amount int) (int, error) {
	s.total += amount
	return s.total, nil
}
func (s *stubProfiles) ListLeaderboard(context.Context, int) ([]profile.LeaderboardEntry, error) {
	return nil, nil
}
func (s *stubProfiles) LeaderboardRank(context.Context, string) (int, int, error) {
	return 0, 0, profile.ErrNotFound
}

func samplePlaces() []places.Place {
	freeMinutes := 30
	avgRating := 4.5
	return []places.Place{{
		PlaceID:     "11111111-1111-1111-1111-111111111111",
		NameTh:      "สยามพารากอน",
		NameEn:      "Siam Paragon",
		PlaceType:   "shopping_mall",
		Latitude:    13.746,
		Longitude:   100.535,
		AvgRating:   &avgRating,
		ReviewCount: 3,
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
}

func TestList_ReturnsPlaces(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	sample := samplePlaces()
	repo := &stubRepo{places: sample}
	engine := gin.New()
	handler := places.NewHandler(places.HandlerConfig{Repo: repo})
	engine.GET("/api/v1/places", handler.List)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/places", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusOK, w.Code)
	r.Equal("public, max-age=30", w.Header().Get("Cache-Control"))
	r.NotEmpty(w.Header().Get("ETag"))

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
	r.NotNil(body.Data[0].AvgRating)
	r.Equal(4.5, *body.Data[0].AvgRating)
	r.Equal(3, body.Data[0].ReviewCount)
	r.Equal(30, *body.Data[0].ParkingArea[0].Rate[0].FreeMinutes)
	r.Equal(40.0, body.Data[0].ParkingArea[0].Rate[0].RateTier[0].Price)
}

func TestList_UsesShortTTLCache(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	repo := &stubRepo{places: samplePlaces()}
	engine := gin.New()
	handler := places.NewHandler(places.HandlerConfig{Repo: repo})
	engine.GET("/api/v1/places", handler.List)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/places", nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		r.Equal(http.StatusOK, w.Code)
	}

	r.Equal(int32(1), repo.listCalls.Load())
}

func TestList_NotModifiedWhenETagMatches(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	repo := &stubRepo{places: samplePlaces()}
	engine := gin.New()
	handler := places.NewHandler(places.HandlerConfig{Repo: repo})
	engine.GET("/api/v1/places", handler.List)

	first := httptest.NewRecorder()
	engine.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/api/v1/places", nil))
	r.Equal(http.StatusOK, first.Code)
	etag := first.Header().Get("ETag")
	r.NotEmpty(etag)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/places", nil)
	req.Header.Set("If-None-Match", etag)
	second := httptest.NewRecorder()
	engine.ServeHTTP(second, req)

	r.Equal(http.StatusNotModified, second.Code)
	r.Equal(etag, second.Header().Get("ETag"))
	r.Equal("public, max-age=30", second.Header().Get("Cache-Control"))
	r.Empty(second.Body.String())
	r.Equal(int32(1), repo.listCalls.Load())
}

func TestList_RepoError(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	handler := places.NewHandler(places.HandlerConfig{
		Repo: &stubRepo{err: errors.New("db down")},
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
