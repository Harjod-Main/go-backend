package profile_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/RinTanth/go-backend/app/auth/supabaseauth"
	"github.com/RinTanth/go-backend/app/pagination"
	"github.com/RinTanth/go-backend/app/profile"
	"github.com/RinTanth/go-common/app"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type stubRepo struct {
	getProfile  *profile.Profile
	getErr      error
	updateOut   *profile.Profile
	updateErr   error
	events      []profile.CreditEvent
	getCalls    atomic.Int32
	ensureCalls atomic.Int32
	updateCalls atomic.Int32
}

func (s *stubRepo) GetByUserID(context.Context, string) (*profile.Profile, error) {
	s.getCalls.Add(1)
	return s.getProfile, s.getErr
}

func (s *stubRepo) Ensure(context.Context, string, string, profile.OAuthSeed) (*profile.Profile, error) {
	s.ensureCalls.Add(1)
	return s.getProfile, nil
}

func (s *stubRepo) SyncFromOAuth(context.Context, string, string, profile.OAuthSeed) (*profile.Profile, error) {
	s.getCalls.Add(1)
	return s.getProfile, s.getErr
}

func (s *stubRepo) Update(context.Context, string, *string, *string, *string, bool) (*profile.Profile, error) {
	s.updateCalls.Add(1)
	return s.updateOut, s.updateErr
}

func (s *stubRepo) AddCreditPoints(_ context.Context, _ string, in profile.CreditAward) (int, error) {
	return in.Amount, nil
}

func (s *stubRepo) ListCreditEvents(context.Context, string, int, *pagination.Cursor) ([]profile.CreditEvent, *string, error) {
	return s.events, nil, nil
}

func (s *stubRepo) ListLeaderboard(context.Context, int) ([]profile.LeaderboardEntry, error) {
	return nil, nil
}

func (s *stubRepo) LeaderboardRank(context.Context, string) (int, int, error) {
	return 0, 0, profile.ErrNotFound
}

func withClaims() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(supabaseauth.CtxClaimsKey, &supabaseauth.Claims{
			Sub:   "11111111-1111-1111-1111-111111111111",
			Email: "user@example.com",
			Role:  "authenticated",
		})
		c.Next()
	}
}

func TestGetProfile_ReturnsNotFoundWithoutCreating(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	repo := &stubRepo{getErr: profile.ErrNotFound}
	handler := profile.NewHandler(profile.HandlerConfig{Repo: repo})

	engine := gin.New()
	engine.Use(withClaims())
	engine.GET("/api/v1/profile", handler.Get)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/profile", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusNotFound, w.Code)
	r.Equal(int32(1), repo.getCalls.Load())
	r.Equal(int32(0), repo.ensureCalls.Load())

	var body struct {
		Code string `json:"code"`
	}
	r.NoError(json.Unmarshal(w.Body.Bytes(), &body))
	r.Equal(string(app.CodeNotFound), body.Code)
}

func TestUpdateProfile_EnsuresBeforeUpdate(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	out := &profile.Profile{
		UserID:      "11111111-1111-1111-1111-111111111111",
		DisplayName: "Updated Name",
		Username:    "updated-name",
	}
	repo := &stubRepo{updateOut: out}
	handler := profile.NewHandler(profile.HandlerConfig{Repo: repo})

	engine := gin.New()
	engine.Use(withClaims())
	engine.PATCH("/api/v1/profile", handler.Update)

	body := bytes.NewBufferString(`{"displayName":"Updated Name"}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/profile", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusOK, w.Code)
	r.Equal(int32(1), repo.ensureCalls.Load())
	r.Equal(int32(1), repo.updateCalls.Load())
}

func TestUpdateProfile_RejectsOversizedBody(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	repo := &stubRepo{}
	handler := profile.NewHandler(profile.HandlerConfig{Repo: repo})

	engine := gin.New()
	engine.Use(withClaims())
	engine.PATCH("/api/v1/profile", handler.Update)

	body := bytes.NewBufferString(`{"displayName":"` + strings.Repeat("x", 20*1024) + `"}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/profile", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusBadRequest, w.Code)
	r.Equal(int32(0), repo.updateCalls.Load())
}

func TestListCreditHistory_RequiresAuth(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	handler := profile.NewHandler(profile.HandlerConfig{Repo: &stubRepo{}})
	engine := gin.New()
	engine.GET("/api/v1/me/credit-points", handler.ListCreditHistory)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/credit-points", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusUnauthorized, w.Code)
}

func TestListCreditHistory_ReturnsEvents(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	repo := &stubRepo{
		events: []profile.CreditEvent{{
			EventID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			Amount:  50,
			Reason:  "review",
		}},
	}
	handler := profile.NewHandler(profile.HandlerConfig{Repo: repo})
	engine := gin.New()
	engine.Use(withClaims())
	engine.GET("/api/v1/me/credit-points", handler.ListCreditHistory)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/credit-points", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusOK, w.Code)
	var body struct {
		Data profile.CreditEventListResponse `json:"data"`
	}
	r.NoError(json.Unmarshal(w.Body.Bytes(), &body))
	r.Len(body.Data.Events, 1)
	r.Equal("review", body.Data.Events[0].Reason)
	r.Equal(50, body.Data.Events[0].Amount)
}
