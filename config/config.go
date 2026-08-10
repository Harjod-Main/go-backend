package config

import (
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"

	env "github.com/caarlos0/env/v11"
)

// Config holds application configuration loaded from environment variables.
type Config struct {
	Server        Server
	AccessControl AccessControl
	Postgres      Postgres
	Header        Header
	Supabase      Supabase
	GooglePlaces  GooglePlaces
}

type Server struct {
	Hostname string `env:"HOSTNAME"`
	Port     string `env:"PORT,notEmpty"`
	// Comma-separated CIDRs/IPs for gin SetTrustedProxies (X-Forwarded-For / rate-limit ClientIP).
	// Required when ENV=PROD. Empty LOCAL uses loopback only.
	TrustedProxyCIDRs string `env:"TRUSTED_PROXY_CIDRS"`
	// Temporary: expose GET /debug/client-ip to discover LB RemoteAddr for TRUSTED_PROXY_CIDRS.
	// Allowed only outside PROD (LOCAL/DEV/UAT). Refused at startup in PROD.
	EnableDebugClientIP bool `env:"ENABLE_DEBUG_CLIENT_IP"`
}

type AccessControl struct {
	AllowOrigin string `env:"ACCESS_CONTROL_ALLOW_ORIGIN"`
}

type Header struct {
	RefIDHeaderKey string `env:"REF_ID_HEADER_KEY,notEmpty"`
}

// Supabase holds settings for Auth JWT verification and project metadata.
type Supabase struct {
	// ProjectURL example: https://xxxx.supabase.co
	ProjectURL string `env:"SUPABASE_PROJECT_URL,notEmpty"`
	// Audience defaults to "authenticated" for user access tokens.
	Audience string `env:"SUPABASE_JWT_AUDIENCE" envDefault:"authenticated"`
}

// GooglePlaces holds the server-side Places API (New) key used for autocomplete proxying.
type GooglePlaces struct {
	// APIKey is required for /places/autocomplete and /places/details. Keep server-only.
	APIKey string `env:"SECRET_GOOGLE_PLACES_API_KEY"`
}

type Postgres struct {
	// DatabaseURL optional full URL (preferred for Supabase).
	// Example: postgres://postgres:pwd@db.xxx.supabase.co:5432/postgres?sslmode=require
	DatabaseURL string `env:"DATABASE_URL"`
	Host        string `env:"DB_HOST"`
	Port        string `env:"DB_PORT" envDefault:"5432"`
	Name        string `env:"DB_NAME" envDefault:"postgres"`
	User        string `env:"SECRET_DB_USER"`
	Password    string `env:"SECRET_DB_PASSWORD"`
	SSLMode     string `env:"DB_SSLMODE" envDefault:"require"`
}

var once sync.Once
var config Config

func prefix(e string) string {
	if e == "" {
		return ""
	}
	return fmt.Sprintf("%s_", e)
}

func C(envPrefix string) Config {
	once.Do(func() {
		opts := env.Options{
			Prefix: prefix(envPrefix),
		}

		var err error
		config, err = parseEnv[Config](opts)
		if err != nil {
			log.Fatal(err)
		}

		if err := validateFoundation(config); err != nil {
			log.Fatal(err)
		}
	})

	return config
}

func validateFoundation(cfg Config) error {
	if cfg.Supabase.ProjectURL == "" {
		return fmt.Errorf("SUPABASE_PROJECT_URL is required")
	}
	// Auth verifies ES256 via project JWKS only (no shared JWT secret).
	// Postgres is required only when business APIs need it. Auth JWT verify is DB-free.

	origin := strings.TrimSpace(cfg.AccessControl.AllowOrigin)
	if IsProdEnv() {
		if origin == "" || origin == "*" {
			return fmt.Errorf("ACCESS_CONTROL_ALLOW_ORIGIN must be an explicit frontend origin in PROD (refusing *)")
		}
		if cfg.Server.EnableDebugClientIP {
			return fmt.Errorf("ENABLE_DEBUG_CLIENT_IP must be false in PROD; discover TRUSTED_PROXY_CIDRS on DEV/UAT instead")
		}
		if strings.TrimSpace(cfg.Server.TrustedProxyCIDRs) == "" {
			return fmt.Errorf("TRUSTED_PROXY_CIDRS is required in PROD (set your load balancer ingress CIDR, not broad RFC1918); discover RemoteAddr via GET /debug/client-ip on DEV/UAT with ENABLE_DEBUG_CLIENT_IP=true")
		}
		if err := ValidatePostgresSSL(cfg.Postgres); err != nil {
			return err
		}
	}
	return nil
}

// ValidatePostgresSSL enforces TLS for Postgres in PROD.
// DATABASE_URL must include sslmode=require|verify-ca|verify-full (not disable/allow/prefer).
// Discrete DB_* fields use the same rule for DB_SSLMODE (default require).
// No-op outside PROD so local Docker can use sslmode=disable.
func ValidatePostgresSSL(pg Postgres) error {
	if !IsProdEnv() {
		return nil
	}
	if strings.TrimSpace(pg.DatabaseURL) != "" {
		return validateDatabaseURLSSL(pg.DatabaseURL)
	}
	if strings.TrimSpace(pg.Host) == "" {
		return nil
	}
	mode := strings.ToLower(strings.TrimSpace(pg.SSLMode))
	if mode == "" {
		mode = "require"
	}
	return assertSecureSSLMode(mode, "DB_SSLMODE")
}

func validateDatabaseURLSSL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("DATABASE_URL is invalid: %w", err)
	}
	mode := strings.ToLower(strings.TrimSpace(u.Query().Get("sslmode")))
	if mode == "" {
		return fmt.Errorf("DATABASE_URL must include sslmode=require (or verify-ca / verify-full) in PROD")
	}
	return assertSecureSSLMode(mode, "DATABASE_URL sslmode")
}

func assertSecureSSLMode(mode, source string) error {
	switch mode {
	case "require", "verify-ca", "verify-full":
		return nil
	case "disable", "allow", "prefer":
		return fmt.Errorf("%s=%q is not allowed in PROD; use require, verify-ca, or verify-full", source, mode)
	default:
		return fmt.Errorf("%s=%q is not allowed in PROD; use require, verify-ca, or verify-full", source, mode)
	}
}

// ResetForTest clears the singleton so tests can reload config.
func ResetForTest() {
	once = sync.Once{}
	config = Config{}
}
