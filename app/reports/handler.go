package reports

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/RinTanth/go-backend/app/auth/supabaseauth"
	"github.com/RinTanth/go-backend/app/mediaurl"
	"github.com/RinTanth/go-common/app"
	"github.com/RinTanth/go-common/wrapper"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var validIssueCategories = map[string]struct{}{
	"functional":  {},
	"performance": {},
	"crash":       {},
	"data":        {},
	"security":    {},
	"other":       {},
}

var validReviewReasons = map[string]struct{}{
	"spam":      {},
	"adult":     {},
	"offensive": {},
	"other":     {},
}

var validPlaceFeedbackTypes = map[string]struct{}{
	"wrong_price":    {},
	"wrong_entrance": {},
	"closed":         {},
	"other":          {},
}

type HandlerConfig struct {
	Repo Repository
}

type Handler struct {
	repo Repository
}

func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{repo: cfg.Repo}
}

// CreateIssueReport handles POST /api/v1/reports (auth optional).
func (h *Handler) CreateIssueReport(c *gin.Context) {
	var body CreateIssueReportRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[IssueReport]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	category := strings.TrimSpace(strings.ToLower(body.Category))
	description := strings.TrimSpace(body.Description)
	if _, ok := validIssueCategories[category]; !ok || description == "" {
		wrapper.Respond(c, wrapper.ResponseOption[IssueReport]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}
	if len(description) > 4000 {
		wrapper.Respond(c, wrapper.ResponseOption[IssueReport]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}
	if len(body.PhotoURLs) > 5 || !mediaurl.ValidMediaURLs(body.PhotoURLs, mediaurl.MaxURLLen) {
		wrapper.Respond(c, wrapper.ResponseOption[IssueReport]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	report := IssueReport{
		Category:      category,
		Description:   description,
		PhotoURLs:     body.PhotoURLs,
		ReporterEmail: body.ReporterEmail,
	}
	if claims, ok := supabaseauth.ClaimsFromGin(c); ok {
		uid := claims.Sub
		report.UserID = &uid
		if report.ReporterEmail == nil && claims.Email != "" {
			email := claims.Email
			report.ReporterEmail = &email
		}
	}

	if err := h.repo.CreateIssueReport(c.Request.Context(), &report); err != nil {
		slog.Error("create issue report failed", "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[IssueReport]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}

	wrapper.Respond(c, wrapper.ResponseOption[IssueReport]{
		HTTPStatus: http.StatusCreated,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       &report,
	})
}

// CreateReviewReport handles POST /api/v1/reviews/:reviewId/reports (auth required).
func (h *Handler) CreateReviewReport(c *gin.Context) {
	claims, ok := supabaseauth.ClaimsFromGin(c)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[ReviewReport]{
			HTTPStatus: http.StatusUnauthorized,
			Code:       app.CodeUnauthorized,
			Message:    app.MessageUnauthorized,
		})
		return
	}

	reviewID := strings.TrimSpace(c.Param("reviewId"))
	if _, err := uuid.Parse(reviewID); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[ReviewReport]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	var body CreateReviewReportRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[ReviewReport]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	reason := strings.TrimSpace(strings.ToLower(body.Reason))
	if _, ok := validReviewReasons[reason]; !ok {
		wrapper.Respond(c, wrapper.ResponseOption[ReviewReport]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	exists, err := h.repo.ReviewExists(c.Request.Context(), reviewID)
	if err != nil {
		slog.Error("review exists check failed", "review_id", reviewID, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[ReviewReport]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}
	if !exists {
		wrapper.Respond(c, wrapper.ResponseOption[ReviewReport]{
			HTTPStatus: http.StatusNotFound,
			Code:       app.CodeNotFound,
			Message:    app.MessageNotFound,
		})
		return
	}

	uid := claims.Sub
	var detail *string
	if body.Detail != nil {
		trimmed := strings.TrimSpace(*body.Detail)
		if trimmed != "" {
			detail = &trimmed
		}
	}

	report := ReviewReport{
		ReviewID: reviewID,
		UserID:   &uid,
		Reason:   reason,
		Detail:   detail,
	}

	if err := h.repo.CreateReviewReport(c.Request.Context(), &report); err != nil {
		slog.Error("create review report failed", "review_id", reviewID, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[ReviewReport]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}

	wrapper.Respond(c, wrapper.ResponseOption[ReviewReport]{
		HTTPStatus: http.StatusCreated,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       &report,
	})
}

// CreatePlaceFeedback handles POST /api/v1/places/:placeId/feedback (auth optional).
func (h *Handler) CreatePlaceFeedback(c *gin.Context) {
	placeID := strings.TrimSpace(c.Param("placeId"))
	if _, err := uuid.Parse(placeID); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[PlaceFeedback]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	var body CreatePlaceFeedbackRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[PlaceFeedback]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	feedbackType := strings.TrimSpace(strings.ToLower(body.FeedbackType))
	if _, ok := validPlaceFeedbackTypes[feedbackType]; !ok {
		wrapper.Respond(c, wrapper.ResponseOption[PlaceFeedback]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	exists, err := h.repo.PlaceExists(c.Request.Context(), placeID)
	if err != nil {
		slog.Error("place exists check failed", "place_id", placeID, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[PlaceFeedback]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}
	if !exists {
		wrapper.Respond(c, wrapper.ResponseOption[PlaceFeedback]{
			HTTPStatus: http.StatusNotFound,
			Code:       app.CodeNotFound,
			Message:    app.MessageNotFound,
		})
		return
	}
	if len(body.PhotoURLs) > 5 || !mediaurl.ValidMediaURLs(body.PhotoURLs, mediaurl.MaxURLLen) {
		wrapper.Respond(c, wrapper.ResponseOption[PlaceFeedback]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}
	if body.PhotoURL != nil {
		trimmed := strings.TrimSpace(*body.PhotoURL)
		if trimmed == "" {
			body.PhotoURL = nil
		} else if !mediaurl.ValidMediaURLs([]string{trimmed}, mediaurl.MaxURLLen) {
			wrapper.Respond(c, wrapper.ResponseOption[PlaceFeedback]{
				HTTPStatus: http.StatusBadRequest,
				Code:       app.CodeBadRequest,
				Message:    app.MessageBadRequest,
			})
			return
		} else {
			body.PhotoURL = &trimmed
		}
	}

	feedback := PlaceFeedback{
		PlaceID:        placeID,
		FeedbackType:   feedbackType,
		Description:    body.Description,
		ReporterEmail:  body.ReporterEmail,
		PhotoURL:       body.PhotoURL,
		PhotoURLs:      body.PhotoURLs,
		OldValue:       body.OldValue,
		SuggestedValue: body.SuggestedValue,
	}
	if claims, ok := supabaseauth.ClaimsFromGin(c); ok {
		uid := claims.Sub
		feedback.UserID = &uid
		if feedback.ReporterEmail == nil && claims.Email != "" {
			email := claims.Email
			feedback.ReporterEmail = &email
		}
	}

	if err := h.repo.CreatePlaceFeedback(c.Request.Context(), &feedback); err != nil {
		slog.Error("create place feedback failed", "place_id", placeID, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[PlaceFeedback]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}

	wrapper.Respond(c, wrapper.ResponseOption[PlaceFeedback]{
		HTTPStatus: http.StatusCreated,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       &feedback,
	})
}
