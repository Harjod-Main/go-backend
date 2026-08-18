package auth_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/RinTanth/go-backend/app/auth"
	"github.com/RinTanth/go-backend/app/auth/supabaseauth"
	"github.com/RinTanth/go-backend/app/pagination"
	"github.com/RinTanth/go-backend/app/profile"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type stubProfileRepo struct {
	getProfile  *profile.Profile
	getErr      error
	getCalls    atomic.Int32
	ensureCalls atomic.Int32
	updateCalls atomic.Int32
}

func (s *stubProfileRepo) GetByUserID(context.Context, string) (*profile.Profile, error) {
	s.getCalls.Add(1)
	return s.getProfile, s.getErr
}

func (s *stubProfileRepo) Ensure(context.Context, string, string, profile.OAuthSeed) (*profile.Profile, error) {
	s.ensureCalls.Add(1)
	return nil, nil
}

func (s *stubProfileRepo) SyncFromOAuth(context.Context, string, string, profile.OAuthSeed) (*profile.Profile, error) {
	s.getCalls.Add(1)
	return s.getProfile, s.getErr
}

func (s *stubProfileRepo) Update(context.Context, string, *string, *string, *string, bool) (*profile.Profile, error) {
	s.updateCalls.Add(1)
	return nil, nil
}

func (s *stubProfileRepo) AddCreditPoints(_ context.Context, _ string, in profile.CreditAward) (int, error) {
	return in.Amount, nil
}

func (s *stubProfileRepo) ListCreditEvents(context.Context, string, int, *pagination.Cursor) ([]profile.CreditEvent, *string, error) {
	return nil, nil, nil
}

func (s *stubProfileRepo) ListLeaderboard(context.Context, int) ([]profile.LeaderboardEntry, error) {
	return nil, nil
}

func (s *stubProfileRepo) LeaderboardRank(context.Context, string) (int, int, error) {
	return 0, 0, profile.ErrNotFound
}

func TestMe_WithValidToken(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	r.NoError(err)
	kid := "me-kid"
	x, y, err := supabaseauth.JWKPublicCoordsForTest(&privateKey.PublicKey)
	r.NoError(err)

	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{{
				"kid": kid,
				"kty": "EC",
				"alg": "ES256",
				"crv": "P-256",
				"x":   x,
				"y":   y,
			}},
		})
	}))
	t.Cleanup(jwks.Close)

	projectURL := "https://abc.supabase.co"
	verifier, err := supabaseauth.NewVerifier(projectURL, "authenticated")
	r.NoError(err)
	verifier.SetJWKSURLForTest(jwks.URL)
	verifier.SetHTTPClientForTest(jwks.Client())

	now := time.Now().Unix()
	token, err := supabaseauth.SignES256ForTest(privateKey, kid, supabaseauth.Claims{
		Sub:   "11111111-1111-1111-1111-111111111111",
		Iss:   projectURL + "/auth/v1",
		Aud:   "authenticated",
		Exp:   now + 3600,
		Iat:   now,
		Role:  "authenticated",
		Email: "user@example.com",
	})
	r.NoError(err)

	engine := gin.New()
	handler := auth.NewHandler(auth.HandlerConfig{})
	engine.GET("/api/v1/auth/me", supabaseauth.Middleware(verifier), handler.Me)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusOK, w.Code)

	var body struct {
		Data auth.MeResponse `json:"data"`
	}
	r.NoError(json.Unmarshal(w.Body.Bytes(), &body))
	r.Equal("11111111-1111-1111-1111-111111111111", body.Data.UserID)
	r.Equal("user@example.com", body.Data.Email)
	r.Equal("authenticated", body.Data.Role)
}

func TestMe_UnauthorizedWithoutToken(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	verifier, err := supabaseauth.NewVerifier("https://abc.supabase.co", "authenticated")
	r.NoError(err)

	engine := gin.New()
	handler := auth.NewHandler(auth.HandlerConfig{})
	engine.GET("/api/v1/auth/me", supabaseauth.Middleware(verifier), handler.Me)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusUnauthorized, w.Code)
}

func TestMe_DoesNotCreateMissingProfile(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	repo := &stubProfileRepo{getErr: profile.ErrNotFound}
	handler := auth.NewHandler(auth.HandlerConfig{ProfileRepo: repo})

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set(supabaseauth.CtxClaimsKey, &supabaseauth.Claims{
			Sub:   "11111111-1111-1111-1111-111111111111",
			Email: "user@example.com",
			Role:  "authenticated",
		})
		c.Next()
	})
	engine.GET("/api/v1/auth/me", handler.Me)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusOK, w.Code)
	r.Equal(int32(1), repo.getCalls.Load())
	r.Equal(int32(0), repo.ensureCalls.Load(), "GET /me must not create a missing profile")

	var body struct {
		Data auth.MeResponse `json:"data"`
	}
	r.NoError(json.Unmarshal(w.Body.Bytes(), &body))
	r.Equal("11111111-1111-1111-1111-111111111111", body.Data.UserID)
	r.Equal("user@example.com", body.Data.Email)
	r.Empty(body.Data.Username)
}

func TestMe_Returns500WhenOAuthSyncFails(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	repo := &stubProfileRepo{getErr: errors.New("db down")}
	handler := auth.NewHandler(auth.HandlerConfig{ProfileRepo: repo})

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set(supabaseauth.CtxClaimsKey, &supabaseauth.Claims{
			Sub:   "11111111-1111-1111-1111-111111111111",
			Email: "user@example.com",
			Role:  "authenticated",
		})
		c.Next()
	})
	engine.GET("/api/v1/auth/me", handler.Me)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusInternalServerError, w.Code)
}

func TestMe_BackfillsExistingProfileFromOAuth(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	avatar := "https://lh3.googleusercontent.com/a/example"
	repo := &stubProfileRepo{
		getProfile: &profile.Profile{
			UserID:       "11111111-1111-1111-1111-111111111111",
			DisplayName:  "aif912752",
			Username:     "aif912752",
			AvatarURL:    &avatar,
			CreditPoints: 1600,
		},
	}
	handler := auth.NewHandler(auth.HandlerConfig{ProfileRepo: repo})

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set(supabaseauth.CtxClaimsKey, &supabaseauth.Claims{
			Sub:   "11111111-1111-1111-1111-111111111111",
			Email: "aif912752@gmail.com",
			Role:  "authenticated",
			UserMetadata: map[string]any{
				"full_name":  "Harjod Tester",
				"avatar_url": avatar,
			},
		})
		c.Next()
	})
	engine.GET("/api/v1/auth/me", handler.Me)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusOK, w.Code)
	r.Equal(int32(1), repo.getCalls.Load())
	r.Equal(int32(0), repo.ensureCalls.Load())

	var body struct {
		Data auth.MeResponse `json:"data"`
	}
	r.NoError(json.Unmarshal(w.Body.Bytes(), &body))
	r.Equal("aif912752", body.Data.DisplayName)
	r.NotNil(body.Data.AvatarURL)
	r.Equal(avatar, *body.Data.AvatarURL)
	r.Equal(1600, body.Data.CreditPoints)
}
