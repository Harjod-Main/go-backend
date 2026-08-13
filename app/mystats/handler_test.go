package mystats_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RinTanth/go-backend/app/auth/supabaseauth"
	"github.com/RinTanth/go-backend/app/mystats"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type stubRepo struct {
	stats *mystats.Stats
	err   error
}

func (s *stubRepo) CountByUser(_ context.Context, _ string) (*mystats.Stats, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.stats, nil
}

func TestGetMineReturnsCounts(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	handler := mystats.NewHandler(mystats.HandlerConfig{
		Repo: &stubRepo{
			stats: &mystats.Stats{
				ReviewCount:          3,
				PlaceSubmissionCount: 7,
				IssueReportCount:     2,
			},
		},
	})
	engine := gin.New()
	engine.GET("/api/v1/me/stats", func(c *gin.Context) {
		c.Set(supabaseauth.CtxClaimsKey, &supabaseauth.Claims{
			Sub: "11111111-1111-1111-1111-111111111111",
		})
		handler.GetMine(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/stats", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusOK, w.Code)
	var body struct {
		Data mystats.Stats `json:"data"`
	}
	r.NoError(json.Unmarshal(w.Body.Bytes(), &body))
	r.Equal(3, body.Data.ReviewCount)
	r.Equal(7, body.Data.PlaceSubmissionCount)
	r.Equal(2, body.Data.IssueReportCount)
}

func TestGetMineUnauthorized(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	handler := mystats.NewHandler(mystats.HandlerConfig{Repo: &stubRepo{}})
	engine := gin.New()
	engine.GET("/api/v1/me/stats", handler.GetMine)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/stats", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusUnauthorized, w.Code)
}

func TestGetMineRepoError(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	handler := mystats.NewHandler(mystats.HandlerConfig{
		Repo: &stubRepo{err: errors.New("db down")},
	})
	engine := gin.New()
	engine.GET("/api/v1/me/stats", func(c *gin.Context) {
		c.Set(supabaseauth.CtxClaimsKey, &supabaseauth.Claims{
			Sub: "11111111-1111-1111-1111-111111111111",
		})
		handler.GetMine(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/stats", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusInternalServerError, w.Code)
}
