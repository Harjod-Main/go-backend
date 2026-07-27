package router

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"time"

	"github.com/RinTanth/go-backend/config"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newPostgresPool(cfg config.Config) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	dsn, err := postgresDSN(cfg.Postgres)
	if err != nil {
		return nil, fmt.Errorf("invalid postgres config: %w", err)
	}

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid postgres dsn: %w", err)
	}

	// Supabase transaction pooler (PgBouncer) rejects prepared statements.
	poolConfig.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create postgres pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}
	return pool, nil
}

func postgresDSN(cfg config.Postgres) (string, error) {
	if cfg.DatabaseURL != "" {
		return cfg.DatabaseURL, nil
	}
	if cfg.Host == "" || cfg.User == "" || cfg.Name == "" {
		return "", fmt.Errorf("DB_HOST, SECRET_DB_USER, and DB_NAME are required when DATABASE_URL is empty")
	}

	port := cfg.Port
	if port == "" {
		port = "5432"
	}

	u := &url.URL{
		Scheme: "postgres",
		Host:   cfg.Host + ":" + port,
		Path:   path.Join("/", cfg.Name),
	}
	if cfg.Password != "" {
		u.User = url.UserPassword(cfg.User, cfg.Password)
	} else {
		u.User = url.User(cfg.User)
	}

	q := url.Values{}
	if cfg.SSLMode != "" {
		q.Set("sslmode", cfg.SSLMode)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}
