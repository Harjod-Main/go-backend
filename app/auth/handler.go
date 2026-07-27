package auth

import (
	"github.com/RinTanth/go-backend/app/profile"
)

type HandlerConfig struct {
	ProfileRepo profile.Repository
}

// Handler serves auth HTTP endpoints.
type Handler struct {
	profileRepo profile.Repository
}

func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{
		profileRepo: cfg.ProfileRepo,
	}
}
