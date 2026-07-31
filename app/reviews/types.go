package reviews

import (
	"errors"
	"time"
)

var (
	ErrNotFound = errors.New("review not found")
)

// Review is the DB row shape returned by list queries.
type Review struct {
	ReviewID    string    `json:"review_id"`
	PlaceID     string    `json:"place_id"`
	UserID      string    `json:"user_id"`
	DisplayName string    `json:"display_name"`
	Rating      int       `json:"rating"`
	Description *string   `json:"description,omitempty"`
	PhotoURLs   []string  `json:"photo_urls,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	LikeCount   int       `json:"like_count"`
	LikedByMe   bool      `json:"liked_by_me"`
}

// ReviewLikeResponse is returned after liking/unliking a review.
type ReviewLikeResponse struct {
	ReviewID  string `json:"reviewId"`
	Liked     bool   `json:"liked"`
	LikeCount int    `json:"likeCount"`
}

// ReviewListResponse is the API response for listing reviews.
type ReviewListResponse struct {
	Reviews    []Review `json:"reviews"`
	NextCursor *string  `json:"next_cursor,omitempty"`
	HasMore    bool     `json:"has_more"`
}

// CreateReviewRequest is the POST body for creating a review.
type CreateReviewRequest struct {
	PlaceID     string   `json:"placeId" binding:"required"`
	Rating      int      `json:"rating" binding:"required,min=1,max=5"`
	Description *string  `json:"description"`
	PhotoURLs   []string `json:"photoUrls"`
}

// UpdateReviewRequest is the PATCH body for updating an owned review.
type UpdateReviewRequest struct {
	Rating      int      `json:"rating" binding:"required,min=1,max=5"`
	Description *string  `json:"description"`
	PhotoURLs   []string `json:"photoUrls"`
}
