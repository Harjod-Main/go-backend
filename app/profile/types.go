package profile

import "time"

type Profile struct {
	UserID      string    `json:"userId"`
	DisplayName string    `json:"displayName"`
	Username    string    `json:"username"`
	AvatarURL   *string   `json:"avatarUrl,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type UpdateProfileRequest struct {
	DisplayName *string `json:"displayName"`
	Username    *string `json:"username"`
	AvatarURL   *string `json:"avatarUrl"`
}
