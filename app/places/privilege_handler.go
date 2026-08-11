package places

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/RinTanth/go-backend/app/auth/supabaseauth"
	"github.com/RinTanth/go-backend/app/points"
	"github.com/RinTanth/go-backend/app/notifications"
	"github.com/RinTanth/go-backend/app/profile"
	"github.com/RinTanth/go-common/app"
	"github.com/RinTanth/go-common/wrapper"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	maxStampCorrectionBodyBytes = 32 * 1024
	maxConditionDescriptionLen  = 4000
	maxStampNotesLen            = 2000
	maxStampLocationLen         = 500
)

type updateStampRequest struct {
	Category             string  `json:"category"`
	ConditionDescription string  `json:"condition_description"`
	Notes                *string `json:"notes"`
	Location             *string `json:"location"`
}

// GetPrivileges returns parking stamps, reserved spots, and EV chargers for a place.
func (h *Handler) GetPrivileges(c *gin.Context) {
	placeID := strings.TrimSpace(c.Param("placeId"))
	if _, err := uuid.Parse(placeID); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[PlacePrivileges]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	privileges, err := h.repo.GetPlacePrivileges(c.Request.Context(), placeID)
	if err != nil {
		slog.Error("place privileges failed", "place_id", placeID, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[PlacePrivileges]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}

	if privileges == nil {
		empty := PlacePrivileges{
			ValidationParking: []ValidationParking{},
			ParkingArea:       []PrivilegeArea{},
		}
		privileges = &empty
	}

	wrapper.Respond(c, wrapper.ResponseOption[PlacePrivileges]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       privileges,
	})
}

// GetPrivilegeDetail returns one privilege by kind (stamp|reserve|ev) and id.
func (h *Handler) GetPrivilegeDetail(c *gin.Context) {
	kind := strings.TrimSpace(c.Param("kind"))
	id := strings.TrimSpace(c.Param("id"))
	if _, err := uuid.Parse(id); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[any]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	ctx := c.Request.Context()

	switch kind {
	case "stamp":
		validation, err := h.repo.GetValidation(ctx, id)
		respondPrivilegeDetail(c, kind, id, validation, err)
	case "reserve":
		reserved, err := h.repo.GetReserved(ctx, id)
		respondPrivilegeDetail(c, kind, id, reserved, err)
	case "ev":
		charger, err := h.repo.GetEVCharger(ctx, id)
		respondPrivilegeDetail(c, kind, id, charger, err)
	default:
		wrapper.Respond(c, wrapper.ResponseOption[any]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
	}
}

func respondPrivilegeDetail[T any](c *gin.Context, kind, id string, payload *T, err error) {
	if err != nil {
		slog.Error("privilege detail failed", "kind", kind, "id", id, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[T]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}
	if payload == nil {
		wrapper.Respond(c, wrapper.ResponseOption[T]{
			HTTPStatus: http.StatusNotFound,
			Code:       app.CodeNotFound,
			Message:    app.MessageNotFound,
		})
		return
	}

	wrapper.Respond(c, wrapper.ResponseOption[T]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       payload,
	})
}

// UpdateStamp handles PATCH /api/v1/privileges/stamp/:id (auth required).
func (h *Handler) UpdateStamp(c *gin.Context) {
	claims, ok := supabaseauth.ClaimsFromGin(c)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[StampCorrectionResult]{
			HTTPStatus: http.StatusUnauthorized,
			Code:       app.CodeUnauthorized,
			Message:    app.MessageUnauthorized,
		})
		return
	}

	id := strings.TrimSpace(c.Param("id"))
	if _, err := uuid.Parse(id); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[StampCorrectionResult]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxStampCorrectionBodyBytes)
	var body updateStampRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[StampCorrectionResult]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	validationType, ok := mapStampCategoryToValidationType(body.Category)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[StampCorrectionResult]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	condition := strings.TrimSpace(body.ConditionDescription)
	if len(condition) > maxConditionDescriptionLen {
		wrapper.Respond(c, wrapper.ResponseOption[StampCorrectionResult]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	notes := trimOptional(body.Notes, maxStampNotesLen)
	if body.Notes != nil && notes == nil && strings.TrimSpace(*body.Notes) != "" {
		wrapper.Respond(c, wrapper.ResponseOption[StampCorrectionResult]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}
	location := trimOptional(body.Location, maxStampLocationLen)
	if body.Location != nil && location == nil && strings.TrimSpace(*body.Location) != "" {
		wrapper.Respond(c, wrapper.ResponseOption[StampCorrectionResult]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}
	if condition == "" && location == nil {
		wrapper.Respond(c, wrapper.ResponseOption[StampCorrectionResult]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	if h.profiles != nil {
		seed := profile.OAuthSeedFromMetadata(claims.Email, claims.UserMetadata)
		if _, err := h.profiles.Ensure(c.Request.Context(), claims.Sub, claims.Email, seed); err != nil {
			slog.Error("ensure profile before stamp correction failed", "user_id", claims.Sub, "error", err)
			wrapper.Respond(c, wrapper.ResponseOption[StampCorrectionResult]{
				HTTPStatus: http.StatusInternalServerError,
				Code:       app.CodeInternalError,
				Message:    app.MessageInternalError,
			})
			return
		}
	}

	updated, firstCorrection, err := h.repo.UpdateValidation(c.Request.Context(), id, UpdateValidationInput{
		ValidationType:       validationType,
		ConditionDescription: condition,
		Notes:                notes,
		ValidationLocation:   location,
		ChangedBy:            claims.Sub,
	})
	if err != nil {
		slog.Error("update stamp failed", "validation_id", id, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[StampCorrectionResult]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}
	if updated == nil {
		wrapper.Respond(c, wrapper.ResponseOption[StampCorrectionResult]{
			HTTPStatus: http.StatusNotFound,
			Code:       app.CodeNotFound,
			Message:    app.MessageNotFound,
		})
		return
	}

	pointsAwarded := 0
	if firstCorrection && h.profiles != nil {
		if _, err := h.profiles.AddCreditPoints(c.Request.Context(), claims.Sub, points.PrivilegeCorrection); err != nil {
			slog.Error("award stamp correction points failed", "user_id", claims.Sub, "error", err)
		} else {
			pointsAwarded = points.PrivilegeCorrection
		}
	}

	wrapper.Respond(c, wrapper.ResponseOption[StampCorrectionResult]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data: &StampCorrectionResult{
			Validation:    *updated,
			PointsAwarded: pointsAwarded,
		},
	})
}

func mapStampCategoryToValidationType(category string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(category)) {
	case "SPENDING":
		return "spending", true
	case "ACTIVITY":
		return "event_ticket", true
	case "BANK_CARD":
		return "credential", true
	case "MEMBERSHIP":
		return "membership", true
	case "OTHER":
		return "other", true
	default:
		return "", false
	}
}

type updateReservedRequest struct {
	Category string  `json:"category"`
	Name     string  `json:"name"`
	Rule     *string `json:"rule"`
	Location *string `json:"location"`
}

// UpdateReserved handles PATCH /api/v1/privileges/reserve/:id (auth required).
func (h *Handler) UpdateReserved(c *gin.Context) {
	claims, ok := supabaseauth.ClaimsFromGin(c)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[ReservedCorrectionResult]{
			HTTPStatus: http.StatusUnauthorized,
			Code:       app.CodeUnauthorized,
			Message:    app.MessageUnauthorized,
		})
		return
	}

	id := strings.TrimSpace(c.Param("id"))
	if _, err := uuid.Parse(id); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[ReservedCorrectionResult]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxStampCorrectionBodyBytes)
	var body updateReservedRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[ReservedCorrectionResult]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	reservationType, ok := mapReserveCategoryToReservationType(body.Category)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[ReservedCorrectionResult]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	name := strings.TrimSpace(body.Name)
	if name == "" || len(name) > maxStampLocationLen {
		wrapper.Respond(c, wrapper.ResponseOption[ReservedCorrectionResult]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}
	programOther := name

	rule := trimOptional(body.Rule, maxConditionDescriptionLen)
	if body.Rule != nil && rule == nil && strings.TrimSpace(*body.Rule) != "" {
		wrapper.Respond(c, wrapper.ResponseOption[ReservedCorrectionResult]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}
	floor := trimOptional(body.Location, maxStampLocationLen)
	if body.Location != nil && floor == nil && strings.TrimSpace(*body.Location) != "" {
		wrapper.Respond(c, wrapper.ResponseOption[ReservedCorrectionResult]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	if h.profiles != nil {
		seed := profile.OAuthSeedFromMetadata(claims.Email, claims.UserMetadata)
		if _, err := h.profiles.Ensure(c.Request.Context(), claims.Sub, claims.Email, seed); err != nil {
			slog.Error("ensure profile before reserved correction failed", "user_id", claims.Sub, "error", err)
			wrapper.Respond(c, wrapper.ResponseOption[ReservedCorrectionResult]{
				HTTPStatus: http.StatusInternalServerError,
				Code:       app.CodeInternalError,
				Message:    app.MessageInternalError,
			})
			return
		}
	}

	updated, firstCorrection, err := h.repo.UpdateReserved(c.Request.Context(), id, UpdateReservedInput{
		ReservationType: reservationType,
		ProgramOther:    &programOther,
		Conditions:      rule,
		Floor:           floor,
		ChangedBy:       claims.Sub,
	})
	if err != nil {
		slog.Error("update reserved failed", "reserved_id", id, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[ReservedCorrectionResult]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}
	if updated == nil {
		wrapper.Respond(c, wrapper.ResponseOption[ReservedCorrectionResult]{
			HTTPStatus: http.StatusNotFound,
			Code:       app.CodeNotFound,
			Message:    app.MessageNotFound,
		})
		return
	}

	pointsAwarded := 0
	if firstCorrection && h.profiles != nil {
		if _, err := h.profiles.AddCreditPoints(c.Request.Context(), claims.Sub, points.PrivilegeCorrection); err != nil {
			slog.Error("award reserved correction points failed", "user_id", claims.Sub, "error", err)
		} else {
			pointsAwarded = points.PrivilegeCorrection
		}
	}

	wrapper.Respond(c, wrapper.ResponseOption[ReservedCorrectionResult]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data: &ReservedCorrectionResult{
			Reserved:      *updated,
			PointsAwarded: pointsAwarded,
		},
	})
}

func mapReserveCategoryToReservationType(category string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(category)) {
	case "CREDITCARD_HOLDERS":
		return "cardholder", true
	case "CORPORATE":
		return "tenant", true
	case "MEMBERSHIP":
		return "other", true
	default:
		return "", false
	}
}

type createPrivilegeRequest struct {
	Kind  string          `json:"kind"`
	Value json.RawMessage `json:"value"`
}

// CreatePrivilege handles POST /api/v1/places/:placeId/privileges (auth required).
func (h *Handler) CreatePrivilege(c *gin.Context) {
	claims, ok := supabaseauth.ClaimsFromGin(c)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[any]{
			HTTPStatus: http.StatusUnauthorized,
			Code:       app.CodeUnauthorized,
			Message:    app.MessageUnauthorized,
		})
		return
	}

	placeID := strings.TrimSpace(c.Param("placeId"))
	if _, err := uuid.Parse(placeID); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[any]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxStampCorrectionBodyBytes)
	var body createPrivilegeRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[any]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	kind := strings.ToLower(strings.TrimSpace(body.Kind))
	if kind != "stamp" && kind != "reserve" && kind != "ev" {
		wrapper.Respond(c, wrapper.ResponseOption[any]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}
	if len(body.Value) == 0 || string(body.Value) == "null" {
		wrapper.Respond(c, wrapper.ResponseOption[any]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	area, err := h.repo.GetParkingAreaForPlace(c.Request.Context(), placeID)
	if err != nil {
		slog.Error("lookup parking area for privilege create failed", "place_id", placeID, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[any]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}
	if area == nil {
		wrapper.Respond(c, wrapper.ResponseOption[any]{
			HTTPStatus: http.StatusNotFound,
			Code:       app.CodeNotFound,
			Message:    app.MessageNotFound,
		})
		return
	}

	if h.profiles != nil {
		seed := profile.OAuthSeedFromMetadata(claims.Email, claims.UserMetadata)
		if _, err := h.profiles.Ensure(c.Request.Context(), claims.Sub, claims.Email, seed); err != nil {
			slog.Error("ensure profile before privilege create failed", "user_id", claims.Sub, "error", err)
			wrapper.Respond(c, wrapper.ResponseOption[any]{
				HTTPStatus: http.StatusInternalServerError,
				Code:       app.CodeInternalError,
				Message:    app.MessageInternalError,
			})
			return
		}
	}

	if err := h.repo.CreatePrivilege(c.Request.Context(), CreatePrivilegeInput{
		PlaceID:       placeID,
		ParkingAreaID: area.ParkingAreaID,
		Latitude:      area.Latitude,
		Longitude:     area.Longitude,
		UserID:        claims.Sub,
		Kind:          kind,
		Value:         body.Value,
	}); err != nil {
		slog.Error("create privilege failed", "place_id", placeID, "kind", kind, "error", err)
		msg := err.Error()
		if strings.Contains(msg, "invalid ") || strings.Contains(msg, "unsupported ") || strings.Contains(msg, "missing ") {
			wrapper.Respond(c, wrapper.ResponseOption[any]{
				HTTPStatus: http.StatusBadRequest,
				Code:       app.CodeBadRequest,
				Message:    app.MessageBadRequest,
			})
			return
		}
		wrapper.Respond(c, wrapper.ResponseOption[any]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}

	created := map[string]bool{"created": true}

	if h.notificationsSender != nil {
		_ = h.notificationsSender.SendToUser(
			c.Request.Context(),
			claims.Sub,
			notifications.NotificationEvent{
				Type:    "contribute",
				PlaceID: placeID,
				Title:   "Thanks for your contribution",
				Body:    "Your parking privileges are now visible on the map.",
				URL:     fmt.Sprintf("/map?placeId=%s", placeID),
			},
		)
	}

	wrapper.Respond(c, wrapper.ResponseOption[map[string]bool]{
		HTTPStatus: http.StatusCreated,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       &created,
	})
}

func trimOptional(value *string, maxLen int) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	if len(trimmed) > maxLen {
		return nil
	}
	return &trimmed
}
