package reports

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/RinTanth/go-backend/app/auth/supabaseauth"
	"github.com/RinTanth/go-backend/app/mediaurl"
	"github.com/RinTanth/go-backend/app/pagination"
	"github.com/RinTanth/go-common/app"
	"github.com/RinTanth/go-common/wrapper"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	maxIssueReportBodyBytes   = 128 * 1024
	maxReviewReportBodyBytes  = 64 * 1024
	maxPlaceFeedbackBodyBytes = 128 * 1024

	maxIssueDescriptionLen   = 4000
	maxReviewReportDetailLen = 4000
	maxPlaceFeedbackTextLen  = 4000
	maxReporterEmailLen      = 254
	maxFeedbackValueFieldLen = 500
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

// ListMine handles GET /api/v1/me/reports (auth required).
func (h *Handler) ListMine(c *gin.Context) {
	claims, ok := supabaseauth.ClaimsFromGin(c)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[IssueReportListResponse]{
			HTTPStatus: http.StatusUnauthorized,
			Code:       app.CodeUnauthorized,
			Message:    app.MessageUnauthorized,
		})
		return
	}

	limit := pagination.ParseLimit(c.Query("limit"), 20, 100)

	var cursor *pagination.Cursor
	if raw := strings.TrimSpace(c.Query("cursor")); raw != "" {
		decoded, err := pagination.Decode(raw)
		if err != nil {
			wrapper.Respond(c, wrapper.ResponseOption[IssueReportListResponse]{
				HTTPStatus: http.StatusBadRequest,
				Code:       app.CodeBadRequest,
				Message:    app.MessageBadRequest,
			})
			return
		}
		cursor = &decoded
	}

	items, nextCursor, err := h.repo.ListIssueReportsByUser(c.Request.Context(), claims.Sub, limit, cursor)
	if err != nil {
		slog.Error("list my issue reports failed", "user_id", claims.Sub, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[IssueReportListResponse]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}
	if items == nil {
		items = []IssueReport{}
	}

	resp := IssueReportListResponse{
		Reports:    items,
		NextCursor: nextCursor,
		HasMore:    nextCursor != nil,
	}
	wrapper.Respond(c, wrapper.ResponseOption[IssueReportListResponse]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       &resp,
	})
}

// CreateIssueReport handles POST /api/v1/reports (auth optional).
func (h *Handler) CreateIssueReport(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxIssueReportBodyBytes)
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
	if len(description) > maxIssueDescriptionLen {
		wrapper.Respond(c, wrapper.ResponseOption[IssueReport]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}
	reporterEmail, ok := normalizeOptionalField(body.ReporterEmail, maxReporterEmailLen)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[IssueReport]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	claims, authenticated := supabaseauth.ClaimsFromGin(c)

	// Issue report photos live in a private bucket, so the client submits object
	// paths instead of public URLs. Uploads are user-scoped; require auth and
	// reject paths outside "{userID}/reports/".
	if len(body.PhotoURLs) > 0 {
		if !authenticated {
			wrapper.Respond(c, wrapper.ResponseOption[IssueReport]{
				HTTPStatus: http.StatusUnauthorized,
				Code:       app.CodeUnauthorized,
				Message:    app.MessageUnauthorized,
			})
			return
		}
		if len(body.PhotoURLs) > 5 || !mediaurl.ValidOwnedPrivateReportPaths(body.PhotoURLs, claims.Sub, mediaurl.MaxURLLen) {
			wrapper.Respond(c, wrapper.ResponseOption[IssueReport]{
				HTTPStatus: http.StatusBadRequest,
				Code:       app.CodeBadRequest,
				Message:    app.MessageBadRequest,
			})
			return
		}
	}

	report := IssueReport{
		Category:      category,
		Description:   description,
		PhotoURLs:     body.PhotoURLs,
		ReporterEmail: reporterEmail,
	}
	if authenticated {
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

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxReviewReportBodyBytes)
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
	detail, ok := normalizeOptionalField(body.Detail, maxReviewReportDetailLen)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[ReviewReport]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
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

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxPlaceFeedbackBodyBytes)
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
	description, ok := normalizeOptionalField(body.Description, maxPlaceFeedbackTextLen)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[PlaceFeedback]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}
	reporterEmail, ok := normalizeOptionalField(body.ReporterEmail, maxReporterEmailLen)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[PlaceFeedback]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}
	oldValue, ok := normalizeOptionalField(body.OldValue, maxFeedbackValueFieldLen)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[PlaceFeedback]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}
	suggestedValue, ok := normalizeOptionalField(body.SuggestedValue, maxFeedbackValueFieldLen)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[PlaceFeedback]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
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
		Description:    description,
		ReporterEmail:  reporterEmail,
		PhotoURL:       body.PhotoURL,
		PhotoURLs:      body.PhotoURLs,
		OldValue:       oldValue,
		SuggestedValue: suggestedValue,
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

func normalizeOptionalField(value *string, maxLen int) (*string, bool) {
	if value == nil {
		return nil, true
	}
	trimmed := strings.TrimSpace(*value)
	if len(trimmed) > maxLen {
		return nil, false
	}
	if trimmed == "" {
		return nil, true
	}
	return &trimmed, true
}
