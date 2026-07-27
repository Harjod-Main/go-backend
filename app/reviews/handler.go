package reviews

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/RinTanth/go-backend/app/auth/supabaseauth"
	"github.com/RinTanth/go-common/app"
	"github.com/RinTanth/go-common/wrapper"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type HandlerConfig struct {
	Repo Repository
}

type Handler struct {
	repo Repository
}

func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{repo: cfg.Repo}
}

// ListByPlace handles GET /api/v1/places/:placeId/reviews?limit=&offset=
func (h *Handler) ListByPlace(c *gin.Context) {
	placeID := strings.TrimSpace(c.Param("placeId"))
	if _, err := uuid.Parse(placeID); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[ReviewListResponse]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	limit := 20
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	offset := 0
	if v := c.Query("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	reviews, total, err := h.repo.ListByPlace(c.Request.Context(), placeID, limit, offset)
	if err != nil {
		slog.Error("list reviews failed", "place_id", placeID, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[ReviewListResponse]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}

	resp := ReviewListResponse{Reviews: reviews, TotalCount: total}
	wrapper.Respond(c, wrapper.ResponseOption[ReviewListResponse]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       &resp,
	})
}

// Create handles POST /api/v1/places/:placeId/reviews (auth required)
func (h *Handler) Create(c *gin.Context) {
	claims, ok := supabaseauth.ClaimsFromGin(c)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[Review]{
			HTTPStatus: http.StatusUnauthorized,
			Code:       app.CodeUnauthorized,
			Message:    app.MessageUnauthorized,
		})
		return
	}

	placeID := strings.TrimSpace(c.Param("placeId"))
	if _, err := uuid.Parse(placeID); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[Review]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	var body CreateReviewRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[Review]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	if body.Rating < 1 || body.Rating > 5 {
		wrapper.Respond(c, wrapper.ResponseOption[Review]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    "rating must be between 1 and 5",
		})
		return
	}

	displayName := claims.Email
	if atIdx := strings.Index(displayName, "@"); atIdx > 0 {
		displayName = displayName[:atIdx]
	}

	review := Review{
		PlaceID:     placeID,
		UserID:      claims.Sub,
		DisplayName: displayName,
		Rating:      body.Rating,
		Description: body.Description,
		PhotoURLs:   body.PhotoURLs,
	}

	if err := h.repo.Create(c.Request.Context(), &review); err != nil {
		slog.Error("create review failed", "place_id", placeID, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[Review]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}

	wrapper.Respond(c, wrapper.ResponseOption[Review]{
		HTTPStatus: http.StatusCreated,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       &review,
	})
}
