package supabaseauth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	defaultJWKSCacheTTL = 10 * time.Minute
	defaultHTTPTimeout  = 5 * time.Second
)

// Claims are the subset of Supabase Auth JWT claims we care about.
type Claims struct {
	Sub   string         `json:"sub"`
	Iss   string         `json:"iss"`
	Aud   FlexibleString `json:"aud"`
	Exp   int64          `json:"exp"`
	Iat   int64          `json:"iat"`
	Role  string         `json:"role"`
	Email string         `json:"email"`
}

// FlexibleString accepts either a JSON string or a single-element string array
// (Supabase may emit aud as either form).
type FlexibleString string

func (s *FlexibleString) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*s = ""
		return nil
	}
	if data[0] == '"' {
		var v string
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		*s = FlexibleString(v)
		return nil
	}
	var arr []string
	if err := json.Unmarshal(data, &arr); err != nil {
		return fmt.Errorf("aud: %w", err)
	}
	if len(arr) == 0 {
		*s = ""
		return nil
	}
	*s = FlexibleString(arr[0])
	return nil
}

// Verifier validates Supabase access tokens.
// Prefers ES256 via the project JWKS; falls back to HS256 with the legacy JWT secret.
type Verifier struct {
	secret   []byte
	issuer   string
	audience string
	jwksURL  string
	client   *http.Client
	cacheTTL time.Duration

	mu        sync.RWMutex
	keysByKid map[string]*ecdsa.PublicKey
	fetchedAt time.Time
}

// NewVerifier builds a verifier for the given Supabase project.
// jwtSecret is used only for legacy HS256 tokens (optional but recommended during migration).
func NewVerifier(jwtSecret, projectURL, audience string) (*Verifier, error) {
	if projectURL == "" {
		return nil, errors.New("supabase project url is required")
	}
	if audience == "" {
		audience = "authenticated"
	}

	base := strings.TrimRight(projectURL, "/")
	return &Verifier{
		secret:   []byte(jwtSecret),
		issuer:   base + "/auth/v1",
		audience: audience,
		jwksURL:  base + "/auth/v1/.well-known/jwks.json",
		client: &http.Client{
			Timeout: defaultHTTPTimeout,
		},
		cacheTTL:  defaultJWKSCacheTTL,
		keysByKid: make(map[string]*ecdsa.PublicKey),
	}, nil
}

// SetJWKSURLForTest overrides the JWKS endpoint (unit tests only).
func (v *Verifier) SetJWKSURLForTest(url string) {
	v.jwksURL = url
}

// SetHTTPClientForTest overrides the HTTP client used to fetch JWKS (unit tests only).
func (v *Verifier) SetHTTPClientForTest(client *http.Client) {
	if client != nil {
		v.client = client
	}
}

func (v *Verifier) Verify(token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid jwt format")
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode header: %w", err)
	}

	var header struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("unmarshal header: %w", err)
	}

	switch header.Alg {
	case "ES256":
		if err := v.verifyES256(parts, header.Kid); err != nil {
			return nil, err
		}
	case "HS256":
		if err := v.verifyHS256(parts); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported alg: %s", header.Alg)
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

func (v *Verifier) verifyHS256(parts []string) error {
	if len(v.secret) == 0 {
		return errors.New("hs256 token requires jwt secret")
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

func (v *Verifier) verifyES256(parts []string, kid string) error {
	if kid == "" {
		return errors.New("es256 token missing kid")
	}

	key, err := v.publicKey(kid)
	if err != nil {
		return err
	}

	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}
	if len(sig) != 64 {
		return fmt.Errorf("invalid es256 signature length: %d", len(sig))
	}

	hash := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	if !ecdsa.Verify(key, hash[:], r, s) {
		return errors.New("invalid signature")
	}
	return nil
}

func (v *Verifier) publicKey(kid string) (*ecdsa.PublicKey, error) {
	v.mu.RLock()
	key, ok := v.keysByKid[kid]
	fresh := time.Since(v.fetchedAt) < v.cacheTTL
	v.mu.RUnlock()
	if ok && fresh {
		return key, nil
	}

	if err := v.refreshJWKS(); err != nil {
		// If refresh fails but we still have a cached key, use it.
		if ok && key != nil {
			return key, nil
		}
		return nil, err
	}

	v.mu.RLock()
	defer v.mu.RUnlock()
	key, ok = v.keysByKid[kid]
	if !ok {
		return nil, fmt.Errorf("jwks: unknown kid %q", kid)
	}
	return key, nil
}

type jwksResponse struct {
	Keys []jwkKey `json:"keys"`
}

type jwkKey struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
	Use string `json:"use"`
}

func (v *Verifier) refreshJWKS() error {
	req, err := http.NewRequest(http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return fmt.Errorf("jwks request: %w", err)
	}

	res, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("jwks fetch: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return fmt.Errorf("jwks fetch: status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload jwksResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return fmt.Errorf("jwks decode: %w", err)
	}

	next := make(map[string]*ecdsa.PublicKey, len(payload.Keys))
	for _, key := range payload.Keys {
		if key.Kty != "EC" || (key.Alg != "" && key.Alg != "ES256") {
			continue
		}
		if key.Crv != "" && key.Crv != "P-256" {
			continue
		}
		if key.Kid == "" || key.X == "" || key.Y == "" {
			continue
		}
		pub, err := ecPublicKeyFromJWK(key.X, key.Y)
		if err != nil {
			return fmt.Errorf("jwks key %q: %w", key.Kid, err)
		}
		next[key.Kid] = pub
	}

	v.mu.Lock()
	v.keysByKid = next
	v.fetchedAt = time.Now()
	v.mu.Unlock()
	return nil
}

func ecPublicKeyFromJWK(xB64, yB64 string) (*ecdsa.PublicKey, error) {
	xBytes, err := base64.RawURLEncoding.DecodeString(xB64)
	if err != nil {
		return nil, fmt.Errorf("decode x: %w", err)
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(yB64)
	if err != nil {
		return nil, fmt.Errorf("decode y: %w", err)
	}

	curve := elliptic.P256()
	x := new(big.Int).SetBytes(xBytes)
	y := new(big.Int).SetBytes(yBytes)
	if !curve.IsOnCurve(x, y) {
		return nil, errors.New("point not on P-256")
	}
	return &ecdsa.PublicKey{Curve: curve, X: x, Y: y}, nil
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
	if claims.Aud != "" && string(claims.Aud) != v.audience {
		return fmt.Errorf("invalid audience: got %q want %q", claims.Aud, v.audience)
	}
	if claims.Sub == "" {
		return errors.New("missing subject")
	}
	return nil
}

// SignHS256ForTest builds a legacy HS256 token for unit tests.
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

// SignES256ForTest builds an ES256 token for unit tests.
func SignES256ForTest(privateKey *ecdsa.PrivateKey, kid string, claims Claims) (string, error) {
	if privateKey == nil {
		return "", errors.New("private key required")
	}
	headerJSON, _ := json.Marshal(map[string]string{
		"alg": "ES256",
		"typ": "JWT",
		"kid": kid,
	})
	payloadJSON, err := json.Marshal(claims)
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

// JWKPublicCoordsForTest returns base64url X/Y for a public key (unit tests).
func JWKPublicCoordsForTest(pub *ecdsa.PublicKey) (x, y string, err error) {
	if pub == nil || pub.X == nil || pub.Y == nil {
		return "", "", errors.New("public key required")
	}
	size := (pub.Curve.Params().BitSize + 7) / 8
	return base64.RawURLEncoding.EncodeToString(pub.X.FillBytes(make([]byte, size))),
		base64.RawURLEncoding.EncodeToString(pub.Y.FillBytes(make([]byte, size))),
		nil
}
