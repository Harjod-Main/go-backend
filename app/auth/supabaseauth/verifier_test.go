package supabaseauth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type es256Fixture struct {
	verifier   *Verifier
	privateKey *ecdsa.PrivateKey
	kid        string
	projectURL string
}

func newES256Fixture(t *testing.T) es256Fixture {
	t.Helper()
	r := require.New(t)

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	r.NoError(err)
	kid := "test-kid-1"
	x, y, err := JWKPublicCoordsForTest(&privateKey.PublicKey)
	r.NoError(err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
	t.Cleanup(server.Close)

	projectURL := "https://abc.supabase.co"
	verifier, err := NewVerifier(projectURL, "authenticated")
	r.NoError(err)
	verifier.SetJWKSURLForTest(server.URL + "/jwks.json")
	verifier.SetHTTPClientForTest(server.Client())

	return es256Fixture{
		verifier:   verifier,
		privateKey: privateKey,
		kid:        kid,
		projectURL: projectURL,
	}
}

func signES256Raw(privateKey *ecdsa.PrivateKey, kid string, payload any) (string, error) {
	headerJSON, _ := json.Marshal(map[string]string{
		"alg": "ES256",
		"typ": "JWT",
		"kid": kid,
	})
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	encodedHeader := base64.RawURLEncoding.EncodeToString(headerJSON)
	encodedPayload := base64.RawURLEncoding.EncodeToString(payloadJSON)
	signingInput := encodedHeader + "." + encodedPayload

	hash := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, privateKey, hash[:])
	if err != nil {
		return "", err
	}

	curveBits := privateKey.Curve.Params().BitSize
	keyBytes := (curveBits + 7) / 8
	sig := make([]byte, 2*keyBytes)
	r.FillBytes(sig[:keyBytes])
	s.FillBytes(sig[keyBytes:])

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func TestVerifier_ValidES256ViaJWKS(t *testing.T) {
	r := require.New(t)
	fx := newES256Fixture(t)

	now := time.Now().Unix()
	token, err := SignES256ForTest(fx.privateKey, fx.kid, Claims{
		Sub:   "22222222-2222-2222-2222-222222222222",
		Iss:   fx.projectURL + "/auth/v1",
		Aud:   "authenticated",
		Exp:   now + 3600,
		Iat:   now,
		Role:  "authenticated",
		Email: "es256@example.com",
	})
	r.NoError(err)

	claims, err := fx.verifier.Verify(token)
	r.NoError(err)
	r.Equal("22222222-2222-2222-2222-222222222222", claims.Sub)
	r.Equal("es256@example.com", claims.Email)
}

func TestVerifier_RejectsHS256(t *testing.T) {
	r := require.New(t)
	fx := newES256Fixture(t)

	now := time.Now().Unix()
	headerJSON, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	payloadJSON, _ := json.Marshal(Claims{
		Sub: "u1",
		Iss: fx.projectURL + "/auth/v1",
		Aud: "authenticated",
		Exp: now + 3600,
		Iat: now,
	})
	token := base64.RawURLEncoding.EncodeToString(headerJSON) + "." +
		base64.RawURLEncoding.EncodeToString(payloadJSON) + ".fakesig"

	_, err := fx.verifier.Verify(token)
	r.ErrorContains(err, "HS256")
}

func TestVerifier_RejectsUnknownKid(t *testing.T) {
	r := require.New(t)
	fx := newES256Fixture(t)

	now := time.Now().Unix()
	token, err := SignES256ForTest(fx.privateKey, "missing-kid", Claims{
		Sub: "u1",
		Iss: fx.projectURL + "/auth/v1",
		Aud: "authenticated",
		Exp: now + 3600,
		Iat: now,
	})
	r.NoError(err)

	_, err = fx.verifier.Verify(token)
	r.ErrorContains(err, "unknown kid")
}

func TestVerifier_RejectsBadSignature(t *testing.T) {
	r := require.New(t)
	fx := newES256Fixture(t)

	otherKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	r.NoError(err)

	now := time.Now().Unix()
	token, err := SignES256ForTest(otherKey, fx.kid, Claims{
		Sub: "u1",
		Iss: fx.projectURL + "/auth/v1",
		Aud: "authenticated",
		Exp: now + 3600,
		Iat: now,
	})
	r.NoError(err)

	_, err = fx.verifier.Verify(token)
	r.Error(err)
}

func TestVerifier_RejectsExpired(t *testing.T) {
	r := require.New(t)
	fx := newES256Fixture(t)

	now := time.Now().Unix()
	token, err := SignES256ForTest(fx.privateKey, fx.kid, Claims{
		Sub: "u1",
		Iss: fx.projectURL + "/auth/v1",
		Aud: "authenticated",
		Exp: now - 10,
		Iat: now - 100,
	})
	r.NoError(err)

	_, err = fx.verifier.Verify(token)
	r.ErrorContains(err, "expired")
}

func TestVerifier_RejectsWrongIssuer(t *testing.T) {
	r := require.New(t)
	fx := newES256Fixture(t)

	now := time.Now().Unix()
	token, err := SignES256ForTest(fx.privateKey, fx.kid, Claims{
		Sub: "u1",
		Iss: "https://other.supabase.co/auth/v1",
		Aud: "authenticated",
		Exp: now + 3600,
		Iat: now,
	})
	r.NoError(err)

	_, err = fx.verifier.Verify(token)
	r.ErrorContains(err, "issuer")
}

func TestVerifier_AcceptsAudAsArray(t *testing.T) {
	r := require.New(t)
	fx := newES256Fixture(t)

	now := time.Now().Unix()
	token, err := signES256Raw(fx.privateKey, fx.kid, map[string]any{
		"sub":   "u1",
		"iss":   fx.projectURL + "/auth/v1",
		"aud":   []string{"authenticated"},
		"exp":   now + 3600,
		"iat":   now,
		"role":  "authenticated",
		"email": "a@b.c",
	})
	r.NoError(err)

	claims, err := fx.verifier.Verify(token)
	r.NoError(err)
	r.Equal(FlexibleString("authenticated"), claims.Aud)
	r.Equal("a@b.c", claims.Email)
}

func TestVerifier_RejectsMissingAudience(t *testing.T) {
	r := require.New(t)
	fx := newES256Fixture(t)

	now := time.Now().Unix()
	token, err := SignES256ForTest(fx.privateKey, fx.kid, Claims{
		Sub: "u1",
		Iss: fx.projectURL + "/auth/v1",
		Exp: now + 3600,
		Iat: now,
	})
	r.NoError(err)

	_, err = fx.verifier.Verify(token)
	r.ErrorContains(err, "audience")
}

func TestVerifier_RejectsWrongAudience(t *testing.T) {
	r := require.New(t)
	fx := newES256Fixture(t)

	now := time.Now().Unix()
	token, err := SignES256ForTest(fx.privateKey, fx.kid, Claims{
		Sub: "u1",
		Iss: fx.projectURL + "/auth/v1",
		Aud: "anon",
		Exp: now + 3600,
		Iat: now,
	})
	r.NoError(err)

	_, err = fx.verifier.Verify(token)
	r.ErrorContains(err, "audience")
}

func TestVerifier_UnknownKidDoesNotStampedeJWKS(t *testing.T) {
	r := require.New(t)
	var fetches atomic.Int32

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	r.NoError(err)
	kid := "cached-kid"
	x, y, err := JWKPublicCoordsForTest(&privateKey.PublicKey)
	r.NoError(err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fetches.Add(1)
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
	t.Cleanup(server.Close)

	projectURL := "https://abc.supabase.co"
	verifier, err := NewVerifier(projectURL, "authenticated")
	r.NoError(err)
	verifier.SetJWKSURLForTest(server.URL + "/jwks.json")
	verifier.SetHTTPClientForTest(server.Client())

	now := time.Now().Unix()
	valid, err := SignES256ForTest(privateKey, kid, Claims{
		Sub: "u1",
		Iss: projectURL + "/auth/v1",
		Aud: "authenticated",
		Exp: now + 3600,
		Iat: now,
	})
	r.NoError(err)
	_, err = verifier.Verify(valid)
	r.NoError(err)
	r.Equal(int32(1), fetches.Load())

	for i := 0; i < 20; i++ {
		token, err := SignES256ForTest(privateKey, fmt.Sprintf("rand-kid-%d", i), Claims{
			Sub: "u1",
			Iss: projectURL + "/auth/v1",
			Aud: "authenticated",
			Exp: now + 3600,
			Iat: now,
		})
		r.NoError(err)
		_, err = verifier.Verify(token)
		r.ErrorContains(err, "unknown kid")
	}
	r.Equal(int32(1), fetches.Load())
}

func TestVerifier_RejectsOversizedToken(t *testing.T) {
	r := require.New(t)
	fx := newES256Fixture(t)

	token := strings.Repeat("a", maxJWTBytes) + ".b.c"
	_, err := fx.verifier.Verify(token)
	r.ErrorContains(err, "too large")
}

func TestVerifier_RejectsUnsupportedTyp(t *testing.T) {
	r := require.New(t)
	fx := newES256Fixture(t)

	headerJSON, _ := json.Marshal(map[string]string{
		"alg": "ES256",
		"typ": "JWE",
		"kid": fx.kid,
	})
	payloadJSON, _ := json.Marshal(Claims{
		Sub: "u1",
		Iss: fx.projectURL + "/auth/v1",
		Aud: "authenticated",
		Exp: time.Now().Unix() + 3600,
		Iat: time.Now().Unix(),
	})
	token := base64.RawURLEncoding.EncodeToString(headerJSON) + "." +
		base64.RawURLEncoding.EncodeToString(payloadJSON) + ".fakesig"

	_, err := fx.verifier.Verify(token)
	r.ErrorContains(err, "typ")
}

func TestVerifier_RejectsFutureNbf(t *testing.T) {
	r := require.New(t)
	fx := newES256Fixture(t)

	now := time.Now().Unix()
	token, err := SignES256ForTest(fx.privateKey, fx.kid, Claims{
		Sub: "u1",
		Iss: fx.projectURL + "/auth/v1",
		Aud: "authenticated",
		Exp: now + 3600,
		Iat: now,
		Nbf: now + 3600,
	})
	r.NoError(err)

	_, err = fx.verifier.Verify(token)
	r.ErrorContains(err, "not yet valid")
}

func TestVerifier_RejectsOversizedJWKS(t *testing.T) {
	r := require.New(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"keys":[` + strings.Repeat(" ", maxJWKSBodyBytes) + `]}`))
	}))
	t.Cleanup(server.Close)

	verifier, err := NewVerifier("https://abc.supabase.co", "authenticated")
	r.NoError(err)
	verifier.SetJWKSURLForTest(server.URL + "/jwks.json")
	verifier.SetHTTPClientForTest(server.Client())

	now := time.Now().Unix()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	r.NoError(err)
	token, err := SignES256ForTest(privateKey, "kid", Claims{
		Sub: "u1",
		Iss: "https://abc.supabase.co/auth/v1",
		Aud: "authenticated",
		Exp: now + 3600,
		Iat: now,
	})
	r.NoError(err)

	_, err = verifier.Verify(token)
	r.ErrorContains(err, "too large")
}
