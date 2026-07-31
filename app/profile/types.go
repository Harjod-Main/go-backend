package profile

import "time"

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
