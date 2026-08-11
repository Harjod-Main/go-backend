package places

import (
	"github.com/RinTanth/go-backend/app/notifications"
	"github.com/RinTanth/go-backend/app/profile"
)

type HandlerConfig struct {
	Repo     Repository
	Google   GooglePlacesClient
	Profiles profile.Repository
	// Optional notification sender for privilege creation.
	NotificationsSender *notifications.Sender
}

// Handler serves places HTTP endpoints.
type Handler struct {
	repo      Repository
	google    GooglePlacesClient
	profiles  profile.Repository
	listCache *listMapCache

	notificationsSender *notifications.Sender
}

func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{
		repo:      cfg.Repo,
		google:    cfg.Google,
		profiles:  cfg.Profiles,
		listCache: newListMapCache(listMapPlacesCacheTTL),
		notificationsSender: cfg.NotificationsSender,
	}
}
