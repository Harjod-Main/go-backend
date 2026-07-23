package supabaseauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Claims are the subset of Supabase Auth JWT claims we care about.
type Claims struct {
	Sub   string `json:"sub"`
	Iss   string `json:"iss"`
	Aud   string `json:"aud"`
	Exp   int64  `json:"exp"`
	Iat   int64  `json:"iat"`
	Role  string `json:"role"`
	Email string `json:"email"`
}

// Verifier validates Supabase access tokens (HS256 + project JWT secret).
type Verifier struct {
	secret   []byte
	issuer   string
	audience string
}

func NewVerifier(jwtSecret, projectURL, audience string) (*Verifier, error) {
	if jwtSecret == "" {
		return nil, errors.New("supabase jwt secret is required")
	}
	if projectURL == "" {
		return nil, errors.New("supabase project url is required")
	}
	if audience == "" {
		audience = "authenticated"
	}

	issuer := strings.TrimRight(projectURL, "/") + "/auth/v1"
	return &Verifier{
		secret:   []byte(jwtSecret),
		issuer:   issuer,
		audience: audience,
	}, nil
}

func (v *Verifier) Verify(token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid jwt format")
	}

	if err := v.verifySignature(parts); err != nil {
		return nil, err
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}

	var claims Claims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("unmarshal claims: %w", err)
	}

	if err := v.validateClaims(&claims); err != nil {
		return nil, err
	}

	return &claims, nil
}

func (v *Verifier) verifySignature(parts []string) error {
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return fmt.Errorf("decode header: %w", err)
	}

	var header struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return fmt.Errorf("unmarshal header: %w", err)
	}
	if header.Alg != "HS256" {
		return fmt.Errorf("unsupported alg: %s", header.Alg)
	}

	mac := hmac.New(sha256.New, v.secret)
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	expected := mac.Sum(nil)

	got, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}
	if !hmac.Equal(expected, got) {
		return errors.New("invalid signature")
	}
	return nil
}

func (v *Verifier) validateClaims(claims *Claims) error {
	now := time.Now().Unix()
	if claims.Exp == 0 || claims.Exp < now {
		return errors.New("token expired")
	}
	if claims.Iat > now+60 {
		return errors.New("token not yet valid")
	}
	if claims.Iss != v.issuer {
		return fmt.Errorf("invalid issuer: got %q want %q", claims.Iss, v.issuer)
	}
	if claims.Aud != "" && claims.Aud != v.audience {
		return fmt.Errorf("invalid audience: got %q want %q", claims.Aud, v.audience)
	}
	if claims.Sub == "" {
		return errors.New("missing subject")
	}
	return nil
}

// SignHS256ForTest builds a token for unit tests.
func SignHS256ForTest(secret string, claims Claims) (string, error) {
	headerJSON, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	encodedHeader := base64.RawURLEncoding.EncodeToString(headerJSON)
	encodedPayload := base64.RawURLEncoding.EncodeToString(payloadJSON)
	signingInput := encodedHeader + "." + encodedPayload

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signingInput))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + sig, nil
}
