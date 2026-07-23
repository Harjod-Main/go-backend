package supabaseauth

import (
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
