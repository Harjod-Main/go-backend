package reviews

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/RinTanth/go-backend/app/auth/supabaseauth"
	"github.com/RinTanth/go-backend/app/mediaurl"
	"github.com/RinTanth/go-backend/app/pagination"
	"github.com/RinTanth/go-backend/app/points"
	"github.com/RinTanth/go-backend/app/profile"
	"github.com/RinTanth/go-common/app"
	"github.com/RinTanth/go-common/wrapper"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	maxReviewCreateBodyBytes = 128 * 1024
	maxReviewDescriptionLen  = 4000
)

type HandlerConfig struct {
	Repo     Repository
	Profiles profile.Repository
}

type Handler struct {
	repo     Repository
	profiles profile.Repository
}

func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{repo: cfg.Repo, profiles: cfg.Profiles}
}

// ListByPlace handles GET /api/v1/places/:placeId/reviews?limit=&cursor=
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

	limit := pagination.ParseLimit(c.Query("limit"), 20, 100)

	var cursor *pagination.Cursor
	if raw := strings.TrimSpace(c.Query("cursor")); raw != "" {
		decoded, err := pagination.Decode(raw)
		if err != nil {
			wrapper.Respond(c, wrapper.ResponseOption[ReviewListResponse]{
				HTTPStatus: http.StatusBadRequest,
				Code:       app.CodeBadRequest,
				Message:    app.MessageBadRequest,
			})
			return
		}
		cursor = &decoded
	}

	reviews, nextCursor, err := h.repo.ListByPlace(c.Request.Context(), placeID, limit, cursor)
	if err != nil {
		slog.Error("list reviews failed", "place_id", placeID, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[ReviewListResponse]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}

	resp := ReviewListResponse{
		Reviews:    reviews,
		NextCursor: nextCursor,
		HasMore:    nextCursor != nil,
	}
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

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxReviewCreateBodyBytes)
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
	if len(body.PhotoURLs) > 5 || !mediaurl.ValidMediaURLs(body.PhotoURLs, mediaurl.MaxURLLen) {
		wrapper.Respond(c, wrapper.ResponseOption[Review]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}
	var description *string
	if body.Description != nil {
		trimmed := strings.TrimSpace(*body.Description)
		if len(trimmed) > maxReviewDescriptionLen {
			wrapper.Respond(c, wrapper.ResponseOption[Review]{
				HTTPStatus: http.StatusBadRequest,
				Code:       app.CodeBadRequest,
				Message:    app.MessageBadRequest,
			})
			return
		}
		if trimmed != "" {
			description = &trimmed
		}
	}

	displayName := h.resolveDisplayName(c, claims)

	review := Review{
		PlaceID:     placeID,
		UserID:      claims.Sub,
		DisplayName: displayName,
		Rating:      body.Rating,
		Description: description,
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

	if h.profiles != nil {
		if _, err := h.profiles.AddCreditPoints(c.Request.Context(), claims.Sub, points.ReviewCreate); err != nil {
			// Review is already persisted; log and continue so the user still gets their review.
			slog.Error("award review points failed", "user_id", claims.Sub, "error", err)
		}
	}

	wrapper.Respond(c, wrapper.ResponseOption[Review]{
		HTTPStatus: http.StatusCreated,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       &review,
	})
}

// Update handles PATCH /api/v1/reviews/:reviewId (auth required, owner only).
func (h *Handler) Update(c *gin.Context) {
	claims, ok := supabaseauth.ClaimsFromGin(c)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[Review]{
			HTTPStatus: http.StatusUnauthorized,
			Code:       app.CodeUnauthorized,
			Message:    app.MessageUnauthorized,
		})
		return
	}

	reviewID := strings.TrimSpace(c.Param("reviewId"))
	if _, err := uuid.Parse(reviewID); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[Review]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxReviewCreateBodyBytes)
	var body UpdateReviewRequest
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
	if len(body.PhotoURLs) > 5 || !mediaurl.ValidMediaURLs(body.PhotoURLs, mediaurl.MaxURLLen) {
		wrapper.Respond(c, wrapper.ResponseOption[Review]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}
	var description *string
	if body.Description != nil {
		trimmed := strings.TrimSpace(*body.Description)
		if len(trimmed) > maxReviewDescriptionLen {
			wrapper.Respond(c, wrapper.ResponseOption[Review]{
				HTTPStatus: http.StatusBadRequest,
				Code:       app.CodeBadRequest,
				Message:    app.MessageBadRequest,
			})
			return
		}
		if trimmed != "" {
			description = &trimmed
		}
	}

	review := Review{
		ReviewID:    reviewID,
		UserID:      claims.Sub,
		Rating:      body.Rating,
		Description: description,
		PhotoURLs:   body.PhotoURLs,
	}

	if err := h.repo.Update(c.Request.Context(), claims.Sub, &review); err != nil {
		if errors.Is(err, ErrNotFound) {
			wrapper.Respond(c, wrapper.ResponseOption[Review]{
				HTTPStatus: http.StatusNotFound,
				Code:       app.CodeNotFound,
				Message:    app.MessageNotFound,
			})
			return
		}
		slog.Error("update review failed", "review_id", reviewID, "user_id", claims.Sub, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[Review]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}

	wrapper.Respond(c, wrapper.ResponseOption[Review]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       &review,
	})
}

// resolveDisplayName prefers the user's profile display name, falling back to
// the email local-part only if no profile repository is wired or the lookup
// fails (matching the existing graceful-degradation pattern elsewhere).
func (h *Handler) resolveDisplayName(c *gin.Context, claims *supabaseauth.Claims) string {
	fallback := claims.Email
	if atIdx := strings.Index(fallback, "@"); atIdx > 0 {
		fallback = fallback[:atIdx]
	}

	if h.profiles == nil {
		return fallback
	}

	seed := profile.OAuthSeedFromMetadata(claims.Email, claims.UserMetadata)
	p, err := h.profiles.Ensure(c.Request.Context(), claims.Sub, claims.Email, seed)
	if err != nil {
		slog.Error("ensure profile before review failed", "user_id", claims.Sub, "error", err)
		return fallback
	}

	displayName := strings.TrimSpace(p.DisplayName)
	if displayName == "" {
		return fallback
	}
	return displayName
}
