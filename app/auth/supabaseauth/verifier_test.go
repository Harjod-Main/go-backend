package supabaseauth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestVerifier_ValidToken(t *testing.T) {
	r := require.New(t)
	secret := "test-supabase-jwt-secret"
	projectURL := "https://abc.supabase.co"
	verifier, err := NewVerifier(secret, projectURL, "authenticated")
	r.NoError(err)

	now := time.Now().Unix()
	token, err := SignHS256ForTest(secret, Claims{
		Sub:   "11111111-1111-1111-1111-111111111111",
		Iss:   "https://abc.supabase.co/auth/v1",
		Aud:   "authenticated",
		Exp:   now + 3600,
		Iat:   now,
		Role:  "authenticated",
		Email: "user@example.com",
	})
	r.NoError(err)

	claims, err := verifier.Verify(token)
	r.NoError(err)
	r.Equal("11111111-1111-1111-1111-111111111111", claims.Sub)
	r.Equal("user@example.com", claims.Email)
}

func TestVerifier_ValidES256ViaJWKS(t *testing.T) {
	r := require.New(t)

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	r.NoError(err)
	kid := "test-kid-1"
	x, y, err := JWKPublicCoordsForTest(&privateKey.PublicKey)
	r.NoError(err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.Equal("/jwks.json", req.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{{
				"kid": kid,
				"kty": "EC",
				"alg": "ES256",
				"crv": "P-256",
				"x":   x,
				"y":   y,
				"use": "sig",
			}},
		})
	}))
	defer server.Close()

	projectURL := "https://abc.supabase.co"
	verifier, err := NewVerifier("unused-hs-secret", projectURL, "authenticated")
	r.NoError(err)
	verifier.SetJWKSURLForTest(server.URL + "/jwks.json")
	verifier.SetHTTPClientForTest(server.Client())

	now := time.Now().Unix()
	token, err := SignES256ForTest(privateKey, kid, Claims{
		Sub:   "22222222-2222-2222-2222-222222222222",
		Iss:   projectURL + "/auth/v1",
		Aud:   "authenticated",
		Exp:   now + 3600,
		Iat:   now,
		Role:  "authenticated",
		Email: "es256@example.com",
	})
	r.NoError(err)

	claims, err := verifier.Verify(token)
	r.NoError(err)
	r.Equal("22222222-2222-2222-2222-222222222222", claims.Sub)
	r.Equal("es256@example.com", claims.Email)
}

func TestVerifier_RejectsUnknownKid(t *testing.T) {
	r := require.New(t)

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	r.NoError(err)
	x, y, err := JWKPublicCoordsForTest(&privateKey.PublicKey)
	r.NoError(err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{{
				"kid": "other-kid",
				"kty": "EC",
				"alg": "ES256",
				"crv": "P-256",
				"x":   x,
				"y":   y,
			}},
		})
	}))
	defer server.Close()

	verifier, err := NewVerifier("secret", "https://abc.supabase.co", "authenticated")
	r.NoError(err)
	verifier.SetJWKSURLForTest(server.URL)
	verifier.SetHTTPClientForTest(server.Client())

	now := time.Now().Unix()
	token, err := SignES256ForTest(privateKey, "missing-kid", Claims{
		Sub: "u1",
		Iss: "https://abc.supabase.co/auth/v1",
		Aud: "authenticated",
		Exp: now + 3600,
		Iat: now,
	})
	r.NoError(err)

	_, err = verifier.Verify(token)
	r.ErrorContains(err, "unknown kid")
}

func TestVerifier_RejectsBadSignature(t *testing.T) {
	r := require.New(t)
	verifier, err := NewVerifier("secret-a", "https://abc.supabase.co", "authenticated")
	r.NoError(err)

	now := time.Now().Unix()
	token, err := SignHS256ForTest("secret-b", Claims{
		Sub: "u1",
		Iss: "https://abc.supabase.co/auth/v1",
		Aud: "authenticated",
		Exp: now + 3600,
		Iat: now,
	})
	r.NoError(err)

	_, err = verifier.Verify(token)
	r.Error(err)
}

func TestVerifier_RejectsExpired(t *testing.T) {
	r := require.New(t)
	secret := "secret"
	verifier, err := NewVerifier(secret, "https://abc.supabase.co", "authenticated")
	r.NoError(err)

	now := time.Now().Unix()
	token, err := SignHS256ForTest(secret, Claims{
		Sub: "u1",
		Iss: "https://abc.supabase.co/auth/v1",
		Aud: "authenticated",
		Exp: now - 10,
		Iat: now - 100,
	})
	r.NoError(err)

	_, err = verifier.Verify(token)
	r.ErrorContains(err, "expired")
}

func TestVerifier_RejectsWrongIssuer(t *testing.T) {
	r := require.New(t)
	secret := "secret"
	verifier, err := NewVerifier(secret, "https://abc.supabase.co", "authenticated")
	r.NoError(err)

	now := time.Now().Unix()
	token, err := SignHS256ForTest(secret, Claims{
		Sub: "u1",
		Iss: "https://other.supabase.co/auth/v1",
		Aud: "authenticated",
		Exp: now + 3600,
		Iat: now,
	})
	r.NoError(err)

	_, err = verifier.Verify(token)
	r.ErrorContains(err, "issuer")
}

func TestVerifier_AcceptsAudAsArray(t *testing.T) {
	r := require.New(t)
	secret := "secret"
	projectURL := "https://abc.supabase.co"
	verifier, err := NewVerifier(secret, projectURL, "authenticated")
	r.NoError(err)

	now := time.Now().Unix()
	headerJSON, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	payload := map[string]any{
		"sub":   "u1",
		"iss":   projectURL + "/auth/v1",
		"aud":   []string{"authenticated"},
		"exp":   now + 3600,
		"iat":   now,
		"role":  "authenticated",
		"email": "a@b.c",
	}
	payloadJSON, err := json.Marshal(payload)
	r.NoError(err)

	encodedHeader := base64.RawURLEncoding.EncodeToString(headerJSON)
	encodedPayload := base64.RawURLEncoding.EncodeToString(payloadJSON)
	signingInput := encodedHeader + "." + encodedPayload
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signingInput))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	token := signingInput + "." + sig

	claims, err := verifier.Verify(token)
	r.NoError(err)
	r.Equal(FlexibleString("authenticated"), claims.Aud)
	r.Equal("a@b.c", claims.Email)
}
