package submissions

import (
	"encoding/json"
	"time"
)

type Submission struct {
	SubmissionID      string          `json:"submissionId"`
	UserID            *string         `json:"userId,omitempty"`
	Name              string          `json:"name"`
	NameTh            *string         `json:"nameTh,omitempty"`
	NameEn            *string         `json:"nameEn,omitempty"`
	GooglePlaceID     *string         `json:"googlePlaceId,omitempty"`
	Address           *string         `json:"address,omitempty"`
	AddressTh         *string         `json:"addressTh,omitempty"`
	AddressEn         *string         `json:"addressEn,omitempty"`
	SubdistrictTh     *string         `json:"subdistrictTh,omitempty"`
	SubdistrictEn     *string         `json:"subdistrictEn,omitempty"`
	DistrictTh        *string         `json:"districtTh,omitempty"`
	DistrictEn        *string         `json:"districtEn,omitempty"`
	ProvinceTh        *string         `json:"provinceTh,omitempty"`
	ProvinceEn        *string         `json:"provinceEn,omitempty"`
	PostalCode        *string         `json:"postalCode,omitempty"`
	Latitude          float64         `json:"latitude"`
	Longitude         float64         `json:"longitude"`
	PlaceType         *string         `json:"placeType,omitempty"`
	Amenities         []string        `json:"amenities,omitempty"`
	PhotoURLs         []string        `json:"photoUrls,omitempty"`
	RatePhotoURLs     []string        `json:"ratePhotoUrls,omitempty"`
	LostTicketFee     *string         `json:"lostTicketFee,omitempty"`
	OvernightFee      *string         `json:"overnightFee,omitempty"`
	FreeMinutes       *int            `json:"freeMinutes,omitempty"`
	OpeningHours      json.RawMessage `json:"openingHours,omitempty"`
	RateTiers         json.RawMessage `json:"rateTiers,omitempty"`
	SpecialConditions []string        `json:"specialConditions,omitempty"`
	ParkingStamps     json.RawMessage `json:"parkingStamps,omitempty"`
	ParkingReserved   json.RawMessage `json:"parkingReserved,omitempty"`
	ParkingEvCharges  json.RawMessage `json:"parkingEvCharges,omitempty"`
	Status            string          `json:"status"`
	PlaceID           *string         `json:"placeId,omitempty"`
	CreatedAt         time.Time       `json:"createdAt"`
}

type CreateSubmissionRequest struct {
	Name              string          `json:"name" binding:"required"`
	NameTh            *string         `json:"nameTh"`
	NameEn            *string         `json:"nameEn"`
	GooglePlaceID     *string         `json:"googlePlaceId"`
	Address           *string         `json:"address"`
	AddressTh         *string         `json:"addressTh"`
	AddressEn         *string         `json:"addressEn"`
	SubdistrictTh     *string         `json:"subdistrictTh"`
	SubdistrictEn     *string         `json:"subdistrictEn"`
	DistrictTh        *string         `json:"districtTh"`
	DistrictEn        *string         `json:"districtEn"`
	ProvinceTh        *string         `json:"provinceTh"`
	ProvinceEn        *string         `json:"provinceEn"`
	PostalCode        *string         `json:"postalCode"`
	Latitude          float64         `json:"latitude" binding:"required"`
	Longitude         float64         `json:"longitude" binding:"required"`
	PlaceType         *string         `json:"placeType"`
	Amenities         []string        `json:"amenities"`
	PhotoURLs         []string        `json:"photoUrls"`
	RatePhotoURLs     []string        `json:"ratePhotoUrls"`
	LostTicketFee     *string         `json:"lostTicketFee"`
	OvernightFee      *string         `json:"overnightFee"`
	FreeMinutes       *int            `json:"freeMinutes"`
	OpeningHours      json.RawMessage `json:"openingHours"`
	RateTiers         json.RawMessage `json:"rateTiers"`
	SpecialConditions []string        `json:"specialConditions"`
	ParkingStamps     json.RawMessage `json:"parkingStamps"`
	ParkingReserved   json.RawMessage `json:"parkingReserved"`
	ParkingEvCharges  json.RawMessage `json:"parkingEvCharges"`
}
