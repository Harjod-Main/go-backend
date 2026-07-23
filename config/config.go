package config

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/RinTanth/go-common/codec"
	env "github.com/caarlos0/env/v11"
)

// Config holds application configuration loaded from environment variables.
type Config struct {
	Server        Server
	AccessControl AccessControl
	Postgres      Postgres
	Header        Header
	Supabase      Supabase
	// Legacy custom-auth fields (optional). Kept for gradual migration.
	JWT          JWT
	GoogleClient GoogleClient
	Aesgcm       Aesgcm
	Hash         Hash
}

type Server struct {
	Hostname string `env:"HOSTNAME"`
	Port     string `env:"PORT,notEmpty"`
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
	// JWTSecret is the project JWT secret (HS256) from Supabase settings.
	JWTSecret string `env:"SECRET_SUPABASE_JWT_SECRET,notEmpty"`
	// Audience defaults to "authenticated" for user access tokens.
	Audience string `env:"SUPABASE_JWT_AUDIENCE" envDefault:"authenticated"`
}

type JWT struct {
	Issuer      string        `env:"JWT_ISSUER"`
	Audience    string        `env:"JWT_AUDIENCE"`
	ExpDuration time.Duration `env:"JWT_EXP_DURATION"`
	PrivateKey  string        `env:"SECRET_JWT_PRIVATE_KEY"`
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

type GoogleClient struct {
	VerifyTokenURL    string `env:"GOOGLE_OAUTH2_VERIFY_TOKEN"`
	GetUserProfileURL string `env:"GOOGLE_OAUTH2_GET_USER_PROFILE"`
	RevokeTokenURL    string `env:"GOOGLE_OAUTH2_REVOKE_TOKEN"`
}

type Aesgcm struct {
	Key string `env:"SECRET_AESGCM_KEY"`
}

type Hash struct {
	Pepper string `env:"SECRET_HASH_PEPPER"`
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

		if config.JWT.PrivateKey != "" {
			base64Coder := codec.NewBase64Coder()
			rawJWTPrivateKey, err := base64Coder.DecodeBase64(config.JWT.PrivateKey)
			if err != nil {
				log.Fatal(err)
			}
			config.JWT.PrivateKey = rawJWTPrivateKey
		}
	})

	return config
}

func validateFoundation(cfg Config) error {
	if cfg.Supabase.ProjectURL == "" {
		return fmt.Errorf("SUPABASE_PROJECT_URL is required")
	}
	if cfg.Supabase.JWTSecret == "" {
		return fmt.Errorf("SECRET_SUPABASE_JWT_SECRET is required")
	}
	// Postgres is required only when business APIs need it. Auth JWT verify is DB-free.
	return nil
}

// ResetForTest clears the singleton so tests can reload config.
func ResetForTest() {
	once = sync.Once{}
	config = Config{}
}
