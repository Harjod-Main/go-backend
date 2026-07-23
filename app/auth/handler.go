package auth

import (
	"github.com/RinTanth/go-backend/app/auth/access"
	"github.com/RinTanth/go-common/aesgcm"
	"github.com/RinTanth/go-common/hash"
	"github.com/RinTanth/go-common/token"
)

type HandlerConfig struct {
	Pg           access.PostgresRepoer
	GoogleClient access.GoogleClienter
	Hash         hash.HashManager
	Aesgcm       aesgcm.Aesgcm
	Token        token.JWTSigner
}

// Handler serves auth HTTP endpoints.
type Handler struct {
	pg           access.PostgresRepoer
	googleClient access.GoogleClienter
	hash         hash.HashManager
	aesgcm       aesgcm.Aesgcm
	token        token.JWTSigner
}

func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{
		pg:           cfg.Pg,
		googleClient: cfg.GoogleClient,
		hash:         cfg.Hash,
		aesgcm:       cfg.Aesgcm,
		token:        cfg.Token,
	}
}
