package auth_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
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
