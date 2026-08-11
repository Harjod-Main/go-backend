package checkins

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/RinTanth/go-backend/app/auth/supabaseauth"
	"github.com/RinTanth/go-backend/app/pagination"
	"github.com/RinTanth/go-backend/app/notifications"
	"github.com/RinTanth/go-backend/app/profile"
	"github.com/RinTanth/go-common/app"
	"github.com/RinTanth/go-common/wrapper"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const maxCheckInCreateBodyBytes = 64 * 1024

type HandlerConfig struct {
	Repo     Repository
	Profiles profile.Repository
	NotificationsSender *notifications.Sender
}

type Handler struct {
	repo     Repository
	profiles profile.Repository
	notificationsSender *notifications.Sender
}

func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{
		repo:                  cfg.Repo,
		profiles:             cfg.Profiles,
		notificationsSender: cfg.NotificationsSender,
	}
}

// ListMine handles GET /api/v1/me/check-ins (auth required).
func (h *Handler) ListMine(c *gin.Context) {
	claims, ok := supabaseauth.ClaimsFromGin(c)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[CheckInListResponse]{
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
			wrapper.Respond(c, wrapper.ResponseOption[CheckInListResponse]{
				HTTPStatus: http.StatusBadRequest,
				Code:       app.CodeBadRequest,
				Message:    app.MessageBadRequest,
			})
			return
		}
		cursor = &decoded
	}

	items, nextCursor, err := h.repo.ListByUser(c.Request.Context(), claims.Sub, limit, cursor)
	if err != nil {
		slog.Error("list my check-ins failed", "user_id", claims.Sub, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[CheckInListResponse]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}
	if items == nil {
		items = []CheckInActivity{}
	}

	resp := CheckInListResponse{
		CheckIns:   items,
		NextCursor: nextCursor,
		HasMore:    nextCursor != nil,
	}
	wrapper.Respond(c, wrapper.ResponseOption[CheckInListResponse]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       &resp,
	})
}

// Create handles POST /api/v1/places/:placeId/check-ins (auth required).
func (h *Handler) Create(c *gin.Context) {
	claims, ok := supabaseauth.ClaimsFromGin(c)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[CheckIn]{
			HTTPStatus: http.StatusUnauthorized,
			Code:       app.CodeUnauthorized,
			Message:    app.MessageUnauthorized,
		})
		return
	}

	placeID := strings.TrimSpace(c.Param("placeId"))
	if _, err := uuid.Parse(placeID); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[CheckIn]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxCheckInCreateBodyBytes)
	var body CreateCheckInRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[CheckIn]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	in, err := NormalizeCreateRequest(body)
	if err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[CheckIn]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}
	in.PlaceID = placeID
	in.UserID = claims.Sub

	exists, err := h.repo.PlaceExists(c.Request.Context(), placeID)
	if err != nil {
		slog.Error("place exists check failed", "place_id", placeID, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[CheckIn]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}
	if !exists {
		wrapper.Respond(c, wrapper.ResponseOption[CheckIn]{
			HTTPStatus: http.StatusNotFound,
			Code:       app.CodeNotFound,
			Message:    app.MessageNotFound,
		})
		return
	}

	if h.profiles != nil {
		seed := profile.OAuthSeedFromMetadata(claims.Email, claims.UserMetadata)
		if _, err := h.profiles.Ensure(c.Request.Context(), claims.Sub, claims.Email, seed); err != nil {
			slog.Error("ensure profile before check-in failed", "user_id", claims.Sub, "error", err)
			wrapper.Respond(c, wrapper.ResponseOption[CheckIn]{
				HTTPStatus: http.StatusInternalServerError,
				Code:       app.CodeInternalError,
				Message:    app.MessageInternalError,
			})
			return
		}
	}

	created, err := h.repo.Create(c.Request.Context(), in)
	if err != nil {
		if errors.Is(err, ErrCooldown) {
			wrapper.Respond(c, wrapper.ResponseOption[CheckIn]{
				HTTPStatus: http.StatusConflict,
				Code:       app.CodeBadRequest,
				Message:    "check-in cooldown active for this place",
			})
			return
		}
		slog.Error("create check-in failed", "place_id", placeID, "user_id", claims.Sub, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[CheckIn]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}

	if h.notificationsSender != nil {
		_ = h.notificationsSender.SendToUser(
			c.Request.Context(),
			claims.Sub,
			notifications.NotificationEvent{
				Type:          "checkin",
				PlaceID:       placeID,
				Title:         "Check-in completed",
				Body:          fmt.Sprintf("You earned %d points.", created.PointsAwarded),
				URL:           fmt.Sprintf("/map?placeId=%s", placeID),
				PointsAwarded: created.PointsAwarded,
			},
		)
	}

	wrapper.Respond(c, wrapper.ResponseOption[CheckIn]{
		HTTPStatus: http.StatusCreated,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       created,
	})
}
