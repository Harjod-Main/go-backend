package places

import "encoding/json"

// Place is the map-list payload: pin/list fields only, no nested hours/rates/images.
type Place struct {
	PlaceID           string   `json:"place_id"`
	NameTh            string   `json:"name_th"`
	NameEn            string   `json:"name_en"`
	PlaceType         string   `json:"place_type"`
	Latitude          float64  `json:"latitude"`
	Longitude         float64  `json:"longitude"`
	AddressTh         *string  `json:"address_th"`
	DistrictTh        *string  `json:"district_th"`
	ProvinceTh        *string  `json:"province_th"`
	PostalCode        *string  `json:"postal_code"`
	PhotoURL          *string  `json:"photo_url,omitempty"`
	AvgRating         *float64 `json:"avg_rating"`
	ReviewCount       int      `json:"review_count"`
	HasEVCharging     *bool    `json:"has_ev_charging"`
	HasValet          *bool    `json:"has_valet"`
	HasCover          *bool    `json:"has_cover"`
	TransitAccess     *bool    `json:"transit_access"`
	TransitAccessType *string  `json:"transit_access_type"`
	TotalSpaces       *int     `json:"total_spaces"`
	FreeMinutes       *int     `json:"free_minutes"`
	MinHourlyRate     *float64 `json:"min_hourly_rate"`
	TodayOpenTime     *string  `json:"today_open_time"`
	TodayCloseTime    *string  `json:"today_close_time"`
	TodayIsClosed     *bool    `json:"today_is_closed"`
}

// MapPlaceCard is hours + gallery for a selected pin (loaded on demand).
type MapPlaceCard struct {
	PlaceID   string   `json:"place_id"`
	PhotoURLs []string `json:"photo_urls"`
	Hours     []Hour   `json:"hours"`
}

type Hour struct {
	DayOfWeek string  `json:"day_of_week"`
	OpenTime  *string `json:"open_time"`
	CloseTime *string `json:"close_time"`
	IsClosed  *bool   `json:"is_closed"`
}

// PlaceRateDetail is the full rate sheet for a place (compatible with frontend fetchParkingRate).
type PlaceRateDetail struct {
	FreeMinutes    *int            `json:"free_minutes"`
	DailyMax       *float64        `json:"daily_max"`
	LostTicketFee  *float64        `json:"lost_ticket_fee"`
	NightRate      *float64        `json:"night_rate"`
	NightStartTime *string         `json:"night_start_time"`
	NightEndTime   *string         `json:"night_end_time"`
	Currency       *string         `json:"currency"`
	Notes          *string         `json:"notes"`
	RateTier       []PlaceRateTier `json:"rate_tier"`
}

type PlaceRateTier struct {
	TierOrder int      `json:"tier_order"`
	Price     float64  `json:"price"`
	Unit      string   `json:"unit"`
	FromHour  float64  `json:"from_hour"`
	ToHour    *float64 `json:"to_hour"`
}

// PlacePrivileges is the nested privilege payload (compatible with frontend privilegeTransform).
type PlacePrivileges struct {
	ValidationParking []ValidationParking `json:"validation_parking"`
	ParkingArea       []PrivilegeArea     `json:"parking_area"`
}

type ValidationParking struct {
	Validation *Validation `json:"validation"`
}

type Validation struct {
	ValidationID         string           `json:"validation_id"`
	PlaceID              string           `json:"place_id,omitempty"`
	ValidationType       string           `json:"validation_type"`
	ConditionDescription string           `json:"condition_description"`
	ValidationLocation   *string          `json:"validation_location"`
	Notes                *string          `json:"notes"`
	ProgramOther         *string          `json:"program_other"`
	Program              *Program         `json:"program"`
	ValidationTier       []ValidationTier `json:"validation_tier"`
	SignagePhotos        []string         `json:"signage_photos"`
}

type ValidationTier struct {
	TierOrder   int     `json:"tier_order"`
	MinSpend    float64 `json:"min_spend"`
	FreeMinutes *int    `json:"free_minutes"`
}

type UpdateValidationInput struct {
	ValidationType       string
	ConditionDescription string
	Notes                *string
	ValidationLocation   *string
	ChangedBy            string
	// SignagePhotos nil = leave existing images; non-nil replaces them (empty clears).
	SignagePhotos *[]string
}

type StampCorrectionResult struct {
	Validation    Validation `json:"validation"`
	PointsAwarded int        `json:"points_awarded"`
}

type UpdateReservedInput struct {
	ReservationType string
	ProgramOther    *string
	Conditions      *string
	Floor           *string
	ChangedBy       string
	// SignagePhotos nil = leave existing images; non-nil replaces them (empty clears).
	SignagePhotos *[]string
}

type ReservedCorrectionResult struct {
	Reserved      Reserved `json:"reserved"`
	PointsAwarded int      `json:"points_awarded"`
}

type UpdateRateInput struct {
	FreeMinutes   *int
	LostTicketFee *float64
	OvernightFee  *float64
	Notes         *string
	RateTiers     []RateTierDraft
	ChangedBy     string
}

type RateTierDraft struct {
	FromHour float64
	ToHour   *float64
	Price    float64
	Unit     string
}

type RateCorrectionResult struct {
	Rate          PlaceRateDetail `json:"rate"`
	PointsAwarded int             `json:"points_awarded"`
}

type UpdateParkingAmenitiesInput struct {
	HasCover          bool
	HasEvCharging     bool
	HasValet          bool
	TotalSpaces       *int
	TransitAccess     bool
	TransitAccessType *string
	ChangedBy         string
}

type ParkingAmenitiesCorrectionResult struct {
	PointsAwarded int `json:"points_awarded"`
}

type Program struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Category string `json:"category"`
}

type PrivilegeArea struct {
	Reserved  []Reserved  `json:"reserved"`
	EVCharger []EVCharger `json:"ev_charger"`
}

type ParkingAreaRef struct {
	ParkingAreaID string
	Latitude      float64
	Longitude     float64
}

type CreatePrivilegeInput struct {
	PlaceID       string
	ParkingAreaID string
	Latitude      float64
	Longitude     float64
	UserID        string
	Kind          string
	Value         json.RawMessage
}

type Reserved struct {
	ReservedID           string   `json:"reserved_id"`
	PlaceID              string   `json:"place_id,omitempty"`
	ReservationType      string   `json:"reservation_type"`
	ReservationTypeOther *string  `json:"reservation_type_other"`
	ProgramOther         *string  `json:"program_other"`
	Floor                *string  `json:"floor"`
	Conditions           *string  `json:"conditions"`
	SpotsCount           *int     `json:"spots_count"`
	AdditionalBenefits   *string  `json:"additional_benefits"`
	Program              *Program `json:"program"`
	SignagePhotos        []string `json:"signage_photos"`
}

type EVCharger struct {
	EVChargerID   string        `json:"ev_charger_id"`
	PlaceID       string        `json:"place_id,omitempty"`
	Floor         *string       `json:"floor"`
	Conditions    *string       `json:"conditions"`
	EVProvider    *EVProvider   `json:"ev_provider"`
	EVConnector   []EVConnector `json:"ev_connector"`
	SignagePhotos []string      `json:"signage_photos"`
}

type EVProvider struct {
	Name string `json:"name"`
}

type EVConnector struct {
	ConnectorType string `json:"connector_type"`
}

type UpdateEVInput struct {
	ProviderName string
	Floor        *string
	Conditions   *string
	Connectors   []EVConnectorDraft
	ChangedBy    string
	// SignagePhotos nil = leave existing images; non-nil replaces them (empty clears).
	SignagePhotos *[]string
}

type EVConnectorDraft struct {
	ConnectorType string
	PowerType     string
	PowerKW       int
}

type EVCorrectionResult struct {
	EVCharger     EVCharger `json:"ev_charger"`
	PointsAwarded int       `json:"points_awarded"`
}

type PlaceReactionKind string

const (
	PlaceReactionLike   PlaceReactionKind = "like"
	PlaceReactionUnlike PlaceReactionKind = "unlike"
)

type PlaceReactionRequest struct {
	Reaction PlaceReactionKind `json:"reaction" binding:"required"`
}

type PlaceReactionResponse struct {
	PlaceID     string             `json:"placeId"`
	MyReaction  *PlaceReactionKind `json:"myReaction,omitempty"`
	LikeCount   int                `json:"likeCount"`
	UnlikeCount int                `json:"unlikeCount"`
}
