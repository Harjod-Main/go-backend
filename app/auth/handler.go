package auth

import (
	"github.com/RinTanth/go-backend/app/auth/access"
	"github.com/RinTanth/go-common/aesgcm"
	"github.com/RinTanth/go-common/hash"
	"github.com/RinTanth/go-common/token"
)

type HandlerConfig struct {
	GoogleClient access.GoogleClient
	Hash         hash.HashManager
	Aesgcm       aesgcm.Aesgcm
	Token        token.JWTSigner
}

type handler struct {
	googleClient access.GoogleClient
	hash         hash.HashManager
	aesgcm       aesgcm.Aesgcm
	token        token.JWTSigner
}

func NewHandler(cfg HandlerConfig) *handler {
	return &handler{
		googleClient: cfg.GoogleClient,
		hash:         cfg.Hash,
		aesgcm:       cfg.Aesgcm,
		token:        cfg.Token,
	}
}
