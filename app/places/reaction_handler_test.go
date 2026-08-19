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
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSetReaction_Success(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	handler := places.NewHandler(places.HandlerConfig{Repo: &stubRepo{}})
	engine := gin.New()
	engine.PUT("/api/v1/places/:placeId/reaction", func(c *gin.Context) {
		c.Set(supabaseauth.CtxClaimsKey, &supabaseauth.Claims{
			Sub: "11111111-1111-1111-1111-111111111111",
		})
		handler.SetReaction(c)
	})

	payload, err := json.Marshal(map[string]any{"reaction": "like"})
	r.NoError(err)
	req := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/places/22222222-2222-2222-2222-222222222222/reaction",
		bytes.NewReader(payload),
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusOK, w.Code)
}

func TestSetReaction_RejectsOversizedBody(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	handler := places.NewHandler(places.HandlerConfig{Repo: &stubRepo{}})
	engine := gin.New()
	engine.PUT("/api/v1/places/:placeId/reaction", func(c *gin.Context) {
		c.Set(supabaseauth.CtxClaimsKey, &supabaseauth.Claims{
			Sub: "11111111-1111-1111-1111-111111111111",
		})
		handler.SetReaction(c)
	})

	body := `{"reaction":"like","pad":"` + strings.Repeat("x", 8*1024) + `"}`
	req := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/places/22222222-2222-2222-2222-222222222222/reaction",
		strings.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusBadRequest, w.Code)
}

func TestSetReaction_UsesAuthenticatedUser(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	repo := &stubRepo{}
	handler := places.NewHandler(places.HandlerConfig{Repo: repo})
	engine := gin.New()
	engine.PUT("/api/v1/places/:placeId/reaction", func(c *gin.Context) {
		c.Set(supabaseauth.CtxClaimsKey, &supabaseauth.Claims{
			Sub: "11111111-1111-1111-1111-111111111111",
		})
		handler.SetReaction(c)
	})

	payload, err := json.Marshal(map[string]any{
		"reaction": "like",
		"userId":   "99999999-9999-9999-9999-999999999999",
	})
	r.NoError(err)
	req := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/places/22222222-2222-2222-2222-222222222222/reaction",
		bytes.NewReader(payload),
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusOK, w.Code)
	r.Equal("11111111-1111-1111-1111-111111111111", repo.lastReactionUserID)
}

func TestClearReaction_UsesAuthenticatedUser(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	repo := &stubRepo{}
	handler := places.NewHandler(places.HandlerConfig{Repo: repo})
	engine := gin.New()
	engine.DELETE("/api/v1/places/:placeId/reaction", func(c *gin.Context) {
		c.Set(supabaseauth.CtxClaimsKey, &supabaseauth.Claims{
			Sub: "11111111-1111-1111-1111-111111111111",
		})
		handler.ClearReaction(c)
	})

	req := httptest.NewRequest(
		http.MethodDelete,
		"/api/v1/places/22222222-2222-2222-2222-222222222222/reaction?userId=99999999-9999-9999-9999-999999999999",
		nil,
	)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusOK, w.Code)
	r.Equal("11111111-1111-1111-1111-111111111111", repo.lastReactionUserID)
}
