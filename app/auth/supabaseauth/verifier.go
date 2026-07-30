package supabaseauth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultJWKSCacheTTL = 10 * time.Minute
	minJWKSCacheTTL     = 30 * time.Second
	maxJWKSCacheTTL     = 24 * time.Hour
	defaultHTTPTimeout  = 5 * time.Second
)

// Claims are the subset of Supabase Auth JWT claims we care about.
type Claims struct {
	Sub          string         `json:"sub"`
	Iss          string         `json:"iss"`
	Aud          FlexibleString `json:"aud"`
	Exp          int64          `json:"exp"`
	Iat          int64          `json:"iat"`
	Role         string         `json:"role"`
	Email        string         `json:"email"`
	UserMetadata map[string]any `json:"user_metadata"`
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

// Verifier validates Supabase access tokens via ES256 + project JWKS only.
// Legacy HS256 (shared JWT secret) is intentionally unsupported — that secret
// could forge tokens if leaked.
type Verifier struct {
	issuer   string
	audience string
	jwksURL  string
	client   *http.Client

	mu              sync.RWMutex
	keysByKid       map[string]*ecdsa.PublicKey
	fetchedAt       time.Time
	expiresAt       time.Time
	refreshAfter    time.Time // proactive refresh threshold (before hard expiry)
	defaultCacheTTL time.Duration

	refreshMu sync.Mutex // singleflight JWKS fetches
}

// NewVerifier builds an ES256 JWKS verifier for the given Supabase project.
func NewVerifier(projectURL, audience string) (*Verifier, error) {
	if projectURL == "" {
		return nil, errors.New("supabase project url is required")
	}
	if audience == "" {
		audience = "authenticated"
	}

	base := strings.TrimRight(projectURL, "/")
	return &Verifier{
		issuer:   base + "/auth/v1",
		audience: audience,
		jwksURL:  base + "/auth/v1/.well-known/jwks.json",
		client: &http.Client{
			Timeout: defaultHTTPTimeout,
		},
		defaultCacheTTL: defaultJWKSCacheTTL,
		keysByKid:       make(map[string]*ecdsa.PublicKey),
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

// SetDefaultJWKSCacheTTLForTest overrides the fallback TTL when Cache-Control is absent (unit tests only).
func (v *Verifier) SetDefaultJWKSCacheTTLForTest(ttl time.Duration) {
	if ttl > 0 {
		v.defaultCacheTTL = ttl
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
		return nil, errors.New("unsupported alg: HS256 (legacy shared-secret tokens are disabled)")
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
	now := time.Now()

	v.mu.RLock()
	key, ok := v.keysByKid[kid]
	hardFresh := !v.expiresAt.IsZero() && now.Before(v.expiresAt)
	softFresh := !v.refreshAfter.IsZero() && now.Before(v.refreshAfter)
	v.mu.RUnlock()

	// Known kid + fully fresh: serve from cache.
	if ok && softFresh {
		return key, nil
	}

	// Known kid + within hard TTL but past proactive refresh: serve stale, refresh in background.
	if ok && hardFresh {
		go v.refreshJWKS(false)
		return key, nil
	}

	// Unknown kid (possible key rotation) or hard-expired cache: synchronous refresh.
	force := !ok
	if err := v.refreshJWKS(force); err != nil {
		// If refresh fails but we still have a cached key for this kid, use it.
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

func (v *Verifier) refreshJWKS(force bool) error {
	v.refreshMu.Lock()
	defer v.refreshMu.Unlock()

	now := time.Now()
	v.mu.RLock()
	stillSoftFresh := !v.refreshAfter.IsZero() && now.Before(v.refreshAfter)
	stillHardFresh := !v.expiresAt.IsZero() && now.Before(v.expiresAt)
	v.mu.RUnlock()

	// Another goroutine may have refreshed while we waited for the lock.
	if !force && stillSoftFresh {
		return nil
	}
	_ = stillHardFresh // hard-fresh soft-stale continues into a fetch below

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

	ttl := clampJWKSCacheTTL(cacheTTLFromHeaders(res.Header, v.defaultCacheTTL))
	fetchedAt := time.Now()
	expiresAt := fetchedAt.Add(ttl)
	refreshAfter := fetchedAt.Add(proactiveRefreshAge(ttl))

	v.mu.Lock()
	v.keysByKid = next
	v.fetchedAt = fetchedAt
	v.expiresAt = expiresAt
	v.refreshAfter = refreshAfter
	v.mu.Unlock()
	return nil
}

func proactiveRefreshAge(ttl time.Duration) time.Duration {
	// Refresh near the end of the TTL window (80% of TTL), with a floor so short TTLs still work.
	skew := ttl / 5
	if skew < 5*time.Second {
		skew = 5 * time.Second
	}
	if skew > time.Minute {
		skew = time.Minute
	}
	age := ttl - skew
	if age < ttl/2 {
		age = ttl / 2
	}
	if age <= 0 {
		return ttl / 2
	}
	return age
}

func clampJWKSCacheTTL(ttl time.Duration) time.Duration {
	if ttl < minJWKSCacheTTL {
		return minJWKSCacheTTL
	}
	if ttl > maxJWKSCacheTTL {
		return maxJWKSCacheTTL
	}
	return ttl
}

// cacheTTLFromHeaders prefers s-maxage, then max-age. Falls back when absent or no-cache/no-store.
func cacheTTLFromHeaders(h http.Header, fallback time.Duration) time.Duration {
	cc := h.Get("Cache-Control")
	if cc == "" {
		return fallback
	}

	var maxAge, sMaxAge int64
	hasMaxAge, hasSMaxAge := false, false
	for _, part := range strings.Split(cc, ",") {
		directive := strings.TrimSpace(strings.ToLower(part))
		switch {
		case directive == "no-store" || directive == "no-cache":
			return minJWKSCacheTTL
		case strings.HasPrefix(directive, "s-maxage="):
			if n, err := strconv.ParseInt(strings.TrimPrefix(directive, "s-maxage="), 10, 64); err == nil && n >= 0 {
				sMaxAge = n
				hasSMaxAge = true
			}
		case strings.HasPrefix(directive, "max-age="):
			if n, err := strconv.ParseInt(strings.TrimPrefix(directive, "max-age="), 10, 64); err == nil && n >= 0 {
				maxAge = n
				hasMaxAge = true
			}
		}
	}

	switch {
	case hasSMaxAge:
		return time.Duration(sMaxAge) * time.Second
	case hasMaxAge:
		return time.Duration(maxAge) * time.Second
	default:
		return fallback
	}
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
	if string(claims.Aud) != v.audience {
		return fmt.Errorf("invalid audience: got %q want %q", claims.Aud, v.audience)
	}
	if claims.Sub == "" {
		return errors.New("missing subject")
	}
	return nil
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
