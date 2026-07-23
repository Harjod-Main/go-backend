package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RinTanth/go-backend/app/auth"
	"github.com/RinTanth/go-backend/app/auth/supabaseauth"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMe_WithValidToken(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	secret := "test-secret"
	projectURL := "https://abc.supabase.co"
	verifier, err := supabaseauth.NewVerifier(secret, projectURL, "authenticated")
	r.NoError(err)

	now := time.Now().Unix()
	token, err := supabaseauth.SignHS256ForTest(secret, supabaseauth.Claims{
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

	verifier, err := supabaseauth.NewVerifier("secret", "https://abc.supabase.co", "authenticated")
	r.NoError(err)

	engine := gin.New()
	handler := auth.NewHandler(auth.HandlerConfig{})
	engine.GET("/api/v1/auth/me", supabaseauth.Middleware(verifier), handler.Me)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusUnauthorized, w.Code)
}
