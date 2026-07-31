package checkins

import (
	"errors"
	"time"
)

const (
	PointsCheckIn      = 50
	PointsOccupancy    = 50
	TotalPointsAwarded = PointsCheckIn + PointsOccupancy
	Cooldown         = 6 * time.Hour
	MaxCommentLen      = 4000
)

var (
	ErrNotFound     = errors.New("place not found")
	ErrCooldown   = errors.New("check-in cooldown active")
	ErrInvalidInput = errors.New("invalid check-in input")
)

var validOccupancy = map[string]struct{}{
	"full":       {},
	"crowded":    {},
	"normal":     {},
	"many_space": {},
}

var validEditSuggestions = map[string]struct{}{
	"incorrect_name":    {},
	"incorrect_address": {},
	"incorrect_hours":   {},
	"other":             {},
}

type PointsBreakdown struct {
	CheckIn   int `json:"checkIn"`
	Occupancy int `json:"occupancy"`
}

type CheckIn struct {
	CheckInID       string          `json:"checkInId"`
	PlaceID         string          `json:"placeId"`
	UserID          string          `json:"userId"`
	Occupancy       string          `json:"occupancy"`
	Satisfied       bool            `json:"satisfied"`
	EditSuggestion  *string         `json:"editSuggestion,omitempty"`
	Comment         *string         `json:"comment,omitempty"`
	PointsAwarded   int             `json:"pointsAwarded"`
	PointsBreakdown PointsBreakdown `json:"pointsBreakdown"`
	CreditPoints    int             `json:"creditPoints"`
	CreatedAt       time.Time       `json:"createdAt"`
}

type CreateCheckInRequest struct {
	Occupancy      string  `json:"occupancy" binding:"required"`
	Satisfied      *bool   `json:"satisfied" binding:"required"`
	EditSuggestion *string `json:"editSuggestion"`
	Comment        *string `json:"comment"`
}

type CreateInput struct {
	PlaceID        string
	UserID         string
	Occupancy      string
	Satisfied      bool
	EditSuggestion *string
	Comment        *string
}

// CheckInActivity is a user-facing check-in row for activity feeds.
type CheckInActivity struct {
	CheckInID     string    `json:"checkInId"`
	PlaceID       string    `json:"placeId"`
	PlaceNameTh   string    `json:"placeNameTh"`
	PlaceNameEn   string    `json:"placeNameEn"`
	PointsAwarded int       `json:"pointsAwarded"`
	Occupancy     string    `json:"occupancy"`
	Satisfied     bool      `json:"satisfied"`
	CreatedAt     time.Time `json:"createdAt"`
}

type CheckInListResponse struct {
	CheckIns   []CheckInActivity `json:"checkIns"`
	NextCursor *string           `json:"nextCursor,omitempty"`
	HasMore    bool              `json:"hasMore"`
}
