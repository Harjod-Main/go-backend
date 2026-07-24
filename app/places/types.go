package places

// Place is the map-list payload shape (compatible with frontend PlaceRow / PostgREST).
type Place struct {
	PlaceID      string         `json:"place_id"`
	NameTh       string         `json:"name_th"`
	NameEn       string         `json:"name_en"`
	PlaceType    string         `json:"place_type"`
	Latitude     float64        `json:"latitude"`
	Longitude    float64        `json:"longitude"`
	AddressTh    *string        `json:"address_th"`
	DistrictTh   *string        `json:"district_th"`
	ProvinceTh   *string        `json:"province_th"`
	PostalCode   *string        `json:"postal_code"`
	ParkingArea  []ParkingArea  `json:"parking_area"`
}

type ParkingArea struct {
	TotalSpaces       *int     `json:"total_spaces"`
	HasEVCharging     *bool    `json:"has_ev_charging"`
	HasValet          *bool    `json:"has_valet"`
	HasCover          *bool    `json:"has_cover"`
	TransitAccess     *bool    `json:"transit_access"`
	TransitAccessType *string  `json:"transit_access_type"`
	Hours             []Hour   `json:"hours"`
	Rate              []Rate   `json:"rate"`
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
