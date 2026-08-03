package places

type HandlerConfig struct {
	Repo   Repository
	Google GooglePlacesClient
}

// Handler serves places HTTP endpoints.
type Handler struct {
	repo      Repository
	google    GooglePlacesClient
	listCache *listMapCache
}

func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{
		repo:      cfg.Repo,
		google:    cfg.Google,
		listCache: newListMapCache(listMapPlacesCacheTTL),
	}
}
