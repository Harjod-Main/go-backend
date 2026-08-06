package places

// Place is the map-list payload shape (compatible with frontend PlaceRow / PostgREST).
type Place struct {
	PlaceID     string        `json:"place_id"`
	NameTh      string        `json:"name_th"`
	NameEn      string        `json:"name_en"`
	PlaceType   string        `json:"place_type"`
	Latitude    float64       `json:"latitude"`
	Longitude   float64       `json:"longitude"`
	AddressTh   *string       `json:"address_th"`
	DistrictTh  *string       `json:"district_th"`
	ProvinceTh  *string       `json:"province_th"`
	PostalCode  *string       `json:"postal_code"`
	PhotoURLs   []string      `json:"photo_urls"`
	AvgRating   *float64      `json:"avg_rating"`
	ReviewCount int           `json:"review_count"`
	ParkingArea []ParkingArea `json:"parking_area"`
}

type ParkingArea struct {
	TotalSpaces       *int    `json:"total_spaces"`
	HasEVCharging     *bool   `json:"has_ev_charging"`
	HasValet          *bool   `json:"has_valet"`
	HasCover          *bool   `json:"has_cover"`
	TransitAccess     *bool   `json:"transit_access"`
	TransitAccessType *string `json:"transit_access_type"`
	Hours             []Hour  `json:"hours"`
	Rate              []Rate  `json:"rate"`
}

type Hour struct {
	DayOfWeek string  `json:"day_of_week"`
	OpenTime  *string `json:"open_time"`
	CloseTime *string `json:"close_time"`
	IsClosed  *bool   `json:"is_closed"`
}

type Rate struct {
	FreeMinutes *int       `json:"free_minutes"`
	RateTier    []RateTier `json:"rate_tier"`
}

type RateTier struct {
	TierOrder int     `json:"tier_order"`
	Price     float64 `json:"price"`
	Unit      string  `json:"unit"`
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
	ValidationType       string           `json:"validation_type"`
	ConditionDescription string           `json:"condition_description"`
	ValidationLocation   *string          `json:"validation_location"`
	Notes                *string          `json:"notes"`
	ProgramOther         *string          `json:"program_other"`
	Program              *Program         `json:"program"`
	ValidationTier       []ValidationTier `json:"validation_tier"`
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
}

type StampCorrectionResult struct {
	Validation    Validation `json:"validation"`
	PointsAwarded int        `json:"points_awarded"`
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

type Reserved struct {
	ReservedID           string   `json:"reserved_id"`
	ReservationType      string   `json:"reservation_type"`
	ReservationTypeOther *string  `json:"reservation_type_other"`
	ProgramOther         *string  `json:"program_other"`
	Floor                *string  `json:"floor"`
	Conditions           *string  `json:"conditions"`
	SpotsCount           *int     `json:"spots_count"`
	AdditionalBenefits   *string  `json:"additional_benefits"`
	Program              *Program `json:"program"`
}

type EVCharger struct {
	EVChargerID string        `json:"ev_charger_id"`
	Floor       *string       `json:"floor"`
	EVProvider  *EVProvider   `json:"ev_provider"`
	EVConnector []EVConnector `json:"ev_connector"`
}

type EVProvider struct {
	Name string `json:"name"`
}

type EVConnector struct {
	ConnectorType string `json:"connector_type"`
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
