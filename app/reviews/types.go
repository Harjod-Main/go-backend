package reviews

import "time"

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
}

// ReviewListResponse is the API response for listing reviews.
type ReviewListResponse struct {
	Reviews    []Review `json:"reviews"`
	TotalCount int      `json:"total_count"`
}

// CreateReviewRequest is the POST body for creating a review.
type CreateReviewRequest struct {
	PlaceID     string   `json:"placeId" binding:"required"`
	Rating      int      `json:"rating" binding:"required,min=1,max=5"`
	Description *string  `json:"description"`
	PhotoURLs   []string `json:"photoUrls"`
}
