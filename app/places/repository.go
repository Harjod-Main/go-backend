package places

import "context"

// Repository loads place rows for the map list.
type Repository interface {
	ListMapPlaces(ctx context.Context) ([]Place, error)
	GetPlaceRate(ctx context.Context, placeID string) (*PlaceRateDetail, error)
	// GetPlaceRates loads rate sheets for many places in one query.
	// Missing / blacklisted places are omitted from the map (nil rate).
	GetPlaceRates(ctx context.Context, placeIDs []string) (map[string]*PlaceRateDetail, error)
	GetPlacePrivileges(ctx context.Context, placeID string) (*PlacePrivileges, error)
	GetValidation(ctx context.Context, validationID string) (*Validation, error)
	GetReserved(ctx context.Context, reservedID string) (*Reserved, error)
	GetEVCharger(ctx context.Context, evChargerID string) (*EVCharger, error)
}
