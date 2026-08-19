package supabaseauth

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RinTanth/go-common/app"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func performAuthedPing(t *testing.T, verifier *Verifier, token string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.GET("/ping", Middleware(verifier), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func TestMiddleware_InvalidTokenResponseIsGeneric(t *testing.T) {
	r := require.New(t)
	fx := newES256Fixture(t)
	now := time.Now().Unix()

	hs256Header, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	hs256Payload, _ := json.Marshal(Claims{
		Sub: "u1",
		Iss: fx.projectURL + "/auth/v1",
		Aud: "authenticated",
		Exp: now + 3600,
		Iat: now,
	})
	hs256Token := base64.RawURLEncoding.EncodeToString(hs256Header) + "." +
		base64.RawURLEncoding.EncodeToString(hs256Payload) + ".fakesig"

	wrongIssuer, err := SignES256ForTest(fx.privateKey, fx.kid, Claims{
		Sub: "u1",
		Iss: "https://evil.example/auth/v1",
		Aud: "authenticated",
		Exp: now + 3600,
		Iat: now,
	})
	r.NoError(err)

	jwksLeak := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("jwks-internal-leak"))
	}))
	t.Cleanup(jwksLeak.Close)
	jwksVerifier, err := NewVerifier(fx.projectURL, "authenticated")
	r.NoError(err)
	jwksVerifier.SetJWKSURLForTest(jwksLeak.URL)
	jwksVerifier.SetHTTPClientForTest(jwksLeak.Client())
	jwksToken, err := SignES256ForTest(fx.privateKey, fx.kid, Claims{
		Sub: "u1",
		Iss: fx.projectURL + "/auth/v1",
		Aud: "authenticated",
		Exp: now + 3600,
		Iat: now,
	})
	r.NoError(err)

	cases := []struct {
		name     string
		verifier *Verifier
		token    string
		leaks    []string
	}{
		{name: "malformed", verifier: fx.verifier, token: "not-a-jwt", leaks: []string{"invalid jwt"}},
		{name: "hs256", verifier: fx.verifier, token: hs256Token, leaks: []string{"HS256", "shared-secret"}},
		{name: "issuer", verifier: fx.verifier, token: wrongIssuer, leaks: []string{"invalid issuer", "evil.example", fx.projectURL}},
		{name: "jwks", verifier: jwksVerifier, token: jwksToken, leaks: []string{"jwks", "jwks-internal-leak"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)
			_, verifyErr := tc.verifier.Verify(tc.token)
			r.Error(verifyErr)
			for _, leak := range tc.leaks {
				r.Contains(verifyErr.Error(), leak)
			}

			w := performAuthedPing(t, tc.verifier, tc.token)
			r.Equal(http.StatusUnauthorized, w.Code)

			var body struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}
			r.NoError(json.Unmarshal(w.Body.Bytes(), &body))
			r.Equal(string(app.CodeUnauthorized), body.Code)
			r.Equal(string(app.MessageUnauthorized), body.Message)
			for _, leak := range tc.leaks {
				r.NotContains(w.Body.String(), leak)
			}
		})
	}
}

func TestMiddleware_MissingBearerIsGenericUnauthorized(t *testing.T) {
	r := require.New(t)
	fx := newES256Fixture(t)

	w := performAuthedPing(t, fx.verifier, "")
	r.Equal(http.StatusUnauthorized, w.Code)
	r.Contains(w.Body.String(), string(app.MessageUnauthorized))
	r.NotContains(w.Body.String(), "error")
}
