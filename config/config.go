package config

import (
	"fmt"
	"log"
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
}

type Server struct {
	Hostname string `env:"HOSTNAME"`
	Port     string `env:"PORT,notEmpty"`
	// Comma-separated CIDRs/IPs for gin SetTrustedProxies (X-Forwarded-For parsing).
	// Empty: LOCAL uses loopback; PROD uses private RFC1918/link-local ranges (Render/Fly LB).
	TrustedProxyCIDRs string `env:"TRUSTED_PROXY_CIDRS"`
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
	}
	return nil
}

// ResetForTest clears the singleton so tests can reload config.
func ResetForTest() {
	once = sync.Once{}
	config = Config{}
}
