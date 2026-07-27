package places

type HandlerConfig struct {
	Repo Repository
}

// Handler serves places HTTP endpoints.
type Handler struct {
	repo      Repository
	listCache *listMapCache
}

func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{
		repo:      cfg.Repo,
		listCache: newListMapCache(listMapPlacesCacheTTL),
	}
}
