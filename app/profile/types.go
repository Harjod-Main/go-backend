package profile

import (
	"strings"
	"time"
)

type Profile struct {
	UserID       string    `json:"userId"`
	DisplayName  string    `json:"displayName"`
	Username     string    `json:"username"`
	AvatarURL    *string   `json:"avatarUrl,omitempty"`
	CreditPoints int       `json:"creditPoints"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type LeaderboardEntry struct {
	Rank         int     `json:"rank"`
	UserID       string  `json:"userId"`
	DisplayName  string  `json:"displayName"`
	Username     string  `json:"username"`
	AvatarURL    *string `json:"avatarUrl,omitempty"`
	CreditPoints int     `json:"creditPoints"`
}

type LeaderboardSelf struct {
	Rank         int `json:"rank"`
	CreditPoints int `json:"creditPoints"`
}

type LeaderboardResponse struct {
	Entries []LeaderboardEntry `json:"entries"`
	Self    *LeaderboardSelf   `json:"self,omitempty"`
}

type UpdateProfileRequest struct {
	DisplayName *string `json:"displayName"`
	Username    *string `json:"username"`
	AvatarURL   *string `json:"avatarUrl"`
}

type CreditAward struct {
	Amount     int
	Reason     string
	SourceType string
	SourceID   string
	PlaceID    *string
}

type CreditEvent struct {
	EventID     string    `json:"eventId"`
	Amount      int       `json:"amount"`
	Reason      string    `json:"reason"`
	SourceType  string    `json:"sourceType"`
	SourceID    string    `json:"sourceId"`
	PlaceID     string    `json:"placeId,omitempty"`
	PlaceNameTh string    `json:"placeNameTh"`
	PlaceNameEn string    `json:"placeNameEn"`
	CreatedAt   time.Time `json:"createdAt"`
}

type CreditEventListResponse struct {
	Events     []CreditEvent `json:"events"`
	NextCursor *string       `json:"nextCursor,omitempty"`
	HasMore    bool          `json:"hasMore"`
}

func OptionalPlaceID(id string) *string {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
