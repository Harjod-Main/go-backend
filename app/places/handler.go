package places

import "github.com/RinTanth/go-backend/app/profile"

type HandlerConfig struct {
	Repo     Repository
	Google   GooglePlacesClient
	Profiles profile.Repository
}

// Handler serves places HTTP endpoints.
type Handler struct {
	repo      Repository
	google    GooglePlacesClient
	profiles  profile.Repository
	listCache *listMapCache
}

func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{
		repo:      cfg.Repo,
		google:    cfg.Google,
		profiles:  cfg.Profiles,
		listCache: newListMapCache(listMapPlacesCacheTTL),
	}
}
