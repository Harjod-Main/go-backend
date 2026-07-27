package reports

import "time"

type IssueReport struct {
	ReportID      string    `json:"reportId"`
	UserID        *string   `json:"userId,omitempty"`
	Category      string    `json:"category"`
	Description   string    `json:"description"`
	PhotoURLs     []string  `json:"photoUrls,omitempty"`
	ReporterEmail *string   `json:"reporterEmail,omitempty"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"createdAt"`
}

type CreateIssueReportRequest struct {
	Category      string   `json:"category" binding:"required"`
	Description   string   `json:"description" binding:"required"`
	PhotoURLs     []string `json:"photoUrls"`
	ReporterEmail *string  `json:"reporterEmail"`
}

type ReviewReport struct {
	ReportID  string    `json:"reportId"`
	ReviewID  string    `json:"reviewId"`
	UserID    *string   `json:"userId,omitempty"`
	Reason    string    `json:"reason"`
	Detail    *string   `json:"detail,omitempty"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}

type CreateReviewReportRequest struct {
	Reason string  `json:"reason" binding:"required"`
	Detail *string `json:"detail"`
}

type PlaceFeedback struct {
	FeedbackID     string    `json:"feedbackId"`
	PlaceID        string    `json:"placeId"`
	UserID         *string   `json:"userId,omitempty"`
	FeedbackType   string    `json:"feedbackType"`
	Description    *string   `json:"description,omitempty"`
	ReporterEmail  *string   `json:"reporterEmail,omitempty"`
	PhotoURL       *string   `json:"photoUrl,omitempty"`
	PhotoURLs      []string  `json:"photoUrls,omitempty"`
	OldValue       *string   `json:"oldValue,omitempty"`
	SuggestedValue *string   `json:"suggestedValue,omitempty"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"createdAt"`
}

type CreatePlaceFeedbackRequest struct {
	FeedbackType   string   `json:"feedbackType" binding:"required"`
	Description    *string  `json:"description"`
	ReporterEmail  *string  `json:"reporterEmail"`
	PhotoURL       *string  `json:"photoUrl"`
	PhotoURLs      []string `json:"photoUrls"`
	OldValue       *string  `json:"oldValue"`
	SuggestedValue *string  `json:"suggestedValue"`
}
