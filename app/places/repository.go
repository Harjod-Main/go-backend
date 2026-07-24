package places

import "context"

// Repository loads place rows for the map list.
type Repository interface {
	ListMapPlaces(ctx context.Context) ([]Place, error)
}
