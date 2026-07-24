package auth

import (
	"github.com/RinTanth/go-backend/app/auth/access"
	"github.com/RinTanth/go-common/aesgcm"
	"github.com/RinTanth/go-common/hash"
)

type HandlerConfig struct {
	Pg           access.PostgresRepoer
	GoogleClient access.GoogleClienter
	Hash         hash.HashManager
	Aesgcm       aesgcm.Aesgcm
}

// Handler serves auth HTTP endpoints.
type Handler struct {
	pg           access.PostgresRepoer
	googleClient access.GoogleClienter
	hash         hash.HashManager
	aesgcm       aesgcm.Aesgcm
}

func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{
		pg:           cfg.Pg,
		googleClient: cfg.GoogleClient,
		hash:         cfg.Hash,
		aesgcm:       cfg.Aesgcm,
	}
}
