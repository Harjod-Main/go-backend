package submissions

import (
	"encoding/json"
	"time"
)

type Submission struct {
	SubmissionID      string          `json:"submissionId"`
	UserID            *string         `json:"userId,omitempty"`
	Name              string          `json:"name"`
	Address           *string         `json:"address,omitempty"`
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
	CreatedAt         time.Time       `json:"createdAt"`
}

type CreateSubmissionRequest struct {
	Name              string          `json:"name" binding:"required"`
	Address           *string         `json:"address"`
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
