package places

import (
	"context"
	"errors"
)

var ErrPlaceNotFound = errors.New("place not found")

// Repository loads place rows for the map list.
type Repository interface {
	ListMapPlaces(ctx context.Context, bounds *MapBounds) ([]Place, error)
	RefreshMapPlacePins(ctx context.Context) error
	GetMapPlaceCard(ctx context.Context, placeID string) (*MapPlaceCard, error)
	GetPlaceRate(ctx context.Context, placeID string) (*PlaceRateDetail, error)
	UpdateRate(ctx context.Context, placeID string, in UpdateRateInput) (*PlaceRateDetail, bool, error)
	UpdateParkingAmenities(ctx context.Context, placeID string, in UpdateParkingAmenitiesInput) (*ParkingAmenitiesCorrectionResult, bool, error)
	// GetPlaceRates loads rate sheets for many places in one query.
	// Missing / blacklisted places are omitted from the map (nil rate).
	GetPlaceRates(ctx context.Context, placeIDs []string) (map[string]*PlaceRateDetail, error)
	GetPlacePrivileges(ctx context.Context, placeID string) (*PlacePrivileges, error)
	// WalkInStampFreeMinutes is the best no-spend stamp free minutes per place (-1 = fully free).
	WalkInStampFreeMinutes(ctx context.Context, placeIDs []string) (map[string]int, error)
	GetValidation(ctx context.Context, validationID string) (*Validation, error)
	UpdateValidation(ctx context.Context, validationID string, in UpdateValidationInput) (*Validation, bool, error)
	GetReserved(ctx context.Context, reservedID string) (*Reserved, error)
	UpdateReserved(ctx context.Context, reservedID string, in UpdateReservedInput) (*Reserved, bool, error)
	GetEVCharger(ctx context.Context, evChargerID string) (*EVCharger, error)
	UpdateEVCharger(ctx context.Context, evChargerID string, in UpdateEVInput) (*EVCharger, bool, error)
	GetParkingAreaForPlace(ctx context.Context, placeID string) (*ParkingAreaRef, error)
	CreatePrivilege(ctx context.Context, in CreatePrivilegeInput) error
	PlaceExists(ctx context.Context, placeID string) (bool, error)
	GetPlaceReaction(ctx context.Context, placeID, userID string) (*PlaceReactionResponse, error)
	SetPlaceReaction(ctx context.Context, placeID, userID string, reaction PlaceReactionKind) (*PlaceReactionResponse, error)
	ClearPlaceReaction(ctx context.Context, placeID, userID string) (*PlaceReactionResponse, error)
}
