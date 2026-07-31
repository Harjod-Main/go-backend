package checkins_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RinTanth/go-backend/app/auth/supabaseauth"
	"github.com/RinTanth/go-backend/app/checkins"
	"github.com/RinTanth/go-backend/app/pagination"
	"github.com/RinTanth/go-backend/app/profile"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type stubRepo struct {
	exists       bool
	cooldown     bool
	createCalled bool
	created      *checkins.CheckIn
	listItems    []checkins.CheckInActivity
}

func (s *stubRepo) PlaceExists(_ context.Context, _ string) (bool, error) {
	return s.exists, nil
}

func (s *stubRepo) Create(_ context.Context, in checkins.CreateInput) (*checkins.CheckIn, error) {
	s.createCalled = true
	if s.cooldown {
		return nil, checkins.ErrCooldown
	}
	s.created = &checkins.CheckIn{
		CheckInID:     "cccccccc-cccc-cccc-cccc-cccccccccccc",
		PlaceID:       in.PlaceID,
		UserID:        in.UserID,
		Occupancy:     in.Occupancy,
		Satisfied:     in.Satisfied,
		PointsAwarded: checkins.TotalPointsAwarded,
		PointsBreakdown: checkins.PointsBreakdown{
			CheckIn:   checkins.PointsCheckIn,
			Occupancy: checkins.PointsOccupancy,
		},
		CreditPoints: 100,
		CreatedAt:    time.Now().UTC(),
	}
	return s.created, nil
}

func (s *stubRepo) ListByUser(_ context.Context, _ string, _ int, _ *pagination.Cursor) ([]checkins.CheckInActivity, *string, error) {
	return s.listItems, nil, nil
}

type stubProfiles struct{}

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
func (s *stubProfiles) AddCreditPoints(_ context.Context, _ string, _ int) (int, error) {
	return 0, nil
}
func (s *stubProfiles) ListLeaderboard(context.Context, int) ([]profile.LeaderboardEntry, error) {
	return nil, nil
}
func (s *stubProfiles) LeaderboardRank(context.Context, string) (int, int, error) {
	return 0, 0, profile.ErrNotFound
}

func performCreate(t *testing.T, repo *stubRepo, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	handler := checkins.NewHandler(checkins.HandlerConfig{Repo: repo, Profiles: &stubProfiles{}})
	engine := gin.New()
	engine.POST("/api/v1/places/:placeId/check-ins", func(c *gin.Context) {
		c.Set(supabaseauth.CtxClaimsKey, &supabaseauth.Claims{
			Sub:   "11111111-1111-1111-1111-111111111111",
			Email: "a@example.com",
		})
		handler.Create(c)
	})

	payload, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/places/22222222-2222-2222-2222-222222222222/check-ins",
		bytes.NewReader(payload),
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func TestCreate_Success(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{exists: true}
	satisfied := true
	w := performCreate(t, repo, map[string]any{
		"occupancy": "normal",
		"satisfied": satisfied,
	})
	r.Equal(http.StatusCreated, w.Code)
	r.True(repo.createCalled)
}

func TestCreate_RejectsCooldown(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{exists: true, cooldown: true}
	satisfied := true
	w := performCreate(t, repo, map[string]any{
		"occupancy": "normal",
		"satisfied": satisfied,
	})
	r.Equal(http.StatusConflict, w.Code)
	r.True(repo.createCalled)
}

func TestCreate_RejectsMissingPlace(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{exists: false}
	satisfied := true
	w := performCreate(t, repo, map[string]any{
		"occupancy": "normal",
		"satisfied": satisfied,
	})
	r.Equal(http.StatusNotFound, w.Code)
	r.False(repo.createCalled)
}

func TestCreate_RequiresEditWhenUnsatisfied(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{exists: true}
	satisfied := false
	w := performCreate(t, repo, map[string]any{
		"occupancy": "full",
		"satisfied": satisfied,
	})
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(repo.createCalled)
}

func TestNormalizeCreateRequest(t *testing.T) {
	r := require.New(t)
	satisfied := false
	suggestion := "incorrect_name"
	in, err := checkins.NormalizeCreateRequest(checkins.CreateCheckInRequest{
		Occupancy:      "Crowded",
		Satisfied:      &satisfied,
		EditSuggestion: &suggestion,
	})
	r.NoError(err)
	r.Equal("crowded", in.Occupancy)
	r.False(in.Satisfied)
	r.Equal("incorrect_name", *in.EditSuggestion)
}

func TestListMine_Success(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	repo := &stubRepo{
		listItems: []checkins.CheckInActivity{{
			CheckInID:     "cccccccc-cccc-cccc-cccc-cccccccccccc",
			PlaceID:       "22222222-2222-2222-2222-222222222222",
			PlaceNameTh:   "ลานจอดทดสอบ",
			PlaceNameEn:   "Test Lot",
			PointsAwarded: 100,
			Occupancy:     "normal",
			Satisfied:     true,
			CreatedAt:     time.Date(2026, 7, 30, 7, 0, 0, 0, time.UTC),
		}},
	}
	handler := checkins.NewHandler(checkins.HandlerConfig{Repo: repo})
	engine := gin.New()
	engine.GET("/api/v1/me/check-ins", func(c *gin.Context) {
		c.Set(supabaseauth.CtxClaimsKey, &supabaseauth.Claims{
			Sub: "11111111-1111-1111-1111-111111111111",
		})
		handler.ListMine(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/check-ins?limit=10", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusOK, w.Code)
	var body struct {
		Data checkins.CheckInListResponse `json:"data"`
	}
	r.NoError(json.Unmarshal(w.Body.Bytes(), &body))
	r.False(body.Data.HasMore)
	r.Nil(body.Data.NextCursor)
	r.Len(body.Data.CheckIns, 1)
	r.Equal("Test Lot", body.Data.CheckIns[0].PlaceNameEn)
}
