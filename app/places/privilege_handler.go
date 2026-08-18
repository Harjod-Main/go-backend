package places

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/RinTanth/go-backend/app/auth/supabaseauth"
	"github.com/RinTanth/go-backend/app/mediaurl"
	"github.com/RinTanth/go-backend/app/notifications"
	"github.com/RinTanth/go-backend/app/points"
	"github.com/RinTanth/go-backend/app/profile"
	"github.com/RinTanth/go-common/app"
	"github.com/RinTanth/go-common/wrapper"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	maxStampCorrectionBodyBytes = 64 * 1024
	maxConditionDescriptionLen  = 4000
	maxStampNotesLen            = 2000
	maxStampLocationLen         = 500
	maxPrivilegeSignagePhotos   = 5
)

type updateStampRequest struct {
	Category             string    `json:"category"`
	ConditionDescription string    `json:"condition_description"`
	Notes                *string   `json:"notes"`
	Location             *string   `json:"location"`
	SignagePhotos        *[]string `json:"signagePhotos"`
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

func (h *Handler) ensureProfile(c *gin.Context, claims *supabaseauth.Claims, action string) bool {
	if h.profiles == nil {
		return true
	}
	seed := profile.OAuthSeedFromMetadata(claims.Email, claims.UserMetadata)
	if _, err := h.profiles.Ensure(c.Request.Context(), claims.Sub, claims.Email, seed); err != nil {
		slog.Error("ensure profile before "+action+" failed", "user_id", claims.Sub, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[any]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return false
	}
	return true
}

func (h *Handler) awardCorrectionPoints(c *gin.Context, userID string, firstCorrection bool, sourceType, sourceID, placeID, action string) int {
	if !firstCorrection || h.profiles == nil {
		return 0
	}
	if _, err := h.profiles.AddCreditPoints(c.Request.Context(), userID, profile.CreditAward{
		Amount:     points.PrivilegeCorrection,
		Reason:     points.ReasonCorrection,
		SourceType: sourceType,
		SourceID:   sourceID,
		PlaceID:    profile.OptionalPlaceID(placeID),
	}); err != nil {
		slog.Error("award "+action+" points failed", "user_id", userID, "error", err)
		return 0
	}
	return points.PrivilegeCorrection
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

	signagePhotos, ok := parsePrivilegeSignagePhotos(body.SignagePhotos)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[StampCorrectionResult]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	if !h.ensureProfile(c, claims, "stamp correction") {
		return
	}

	updated, firstCorrection, err := h.repo.UpdateValidation(c.Request.Context(), id, UpdateValidationInput{
		ValidationType:       validationType,
		ConditionDescription: condition,
		Notes:                notes,
		ValidationLocation:   location,
		ChangedBy:            claims.Sub,
		SignagePhotos:        signagePhotos,
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

	pointsAwarded := h.awardCorrectionPoints(c, claims.Sub, firstCorrection, "validation", id, updated.PlaceID, "stamp correction")

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
	Category      string    `json:"category"`
	Name          string    `json:"name"`
	Rule          *string   `json:"rule"`
	Location      *string   `json:"location"`
	SignagePhotos *[]string `json:"signagePhotos"`
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

	signagePhotos, ok := parsePrivilegeSignagePhotos(body.SignagePhotos)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[ReservedCorrectionResult]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	if !h.ensureProfile(c, claims, "reserved correction") {
		return
	}

	updated, firstCorrection, err := h.repo.UpdateReserved(c.Request.Context(), id, UpdateReservedInput{
		ReservationType: reservationType,
		ProgramOther:    &programOther,
		Conditions:      rule,
		Floor:           floor,
		ChangedBy:       claims.Sub,
		SignagePhotos:   signagePhotos,
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

	pointsAwarded := h.awardCorrectionPoints(c, claims.Sub, firstCorrection, "reserved", id, updated.PlaceID, "reserved correction")

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

type updateEVConnectorRequest struct {
	ConnectorType string `json:"connectorType"`
	Total         string `json:"total"`
}

type updateEVRequest struct {
	ProviderName  string                     `json:"providerName"`
	Connectors    []updateEVConnectorRequest `json:"connectors"`
	Rule          *string                    `json:"rule"`
	Location      *string                    `json:"location"`
	SignagePhotos *[]string                  `json:"signagePhotos"`
}

const maxEVConnectors = 20

// UpdateEV handles PATCH /api/v1/privileges/ev/:id (auth required).
func (h *Handler) UpdateEV(c *gin.Context) {
	claims, ok := supabaseauth.ClaimsFromGin(c)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[EVCorrectionResult]{
			HTTPStatus: http.StatusUnauthorized,
			Code:       app.CodeUnauthorized,
			Message:    app.MessageUnauthorized,
		})
		return
	}

	id := strings.TrimSpace(c.Param("id"))
	if _, err := uuid.Parse(id); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[EVCorrectionResult]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxStampCorrectionBodyBytes)
	var body updateEVRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[EVCorrectionResult]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	providerName := normalizeEVProviderName(body.ProviderName)
	if providerName == "" {
		wrapper.Respond(c, wrapper.ResponseOption[EVCorrectionResult]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	connectors, ok := expandEVConnectors(body.Connectors)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[EVCorrectionResult]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	rule := trimOptional(body.Rule, maxConditionDescriptionLen)
	if body.Rule != nil && rule == nil && strings.TrimSpace(*body.Rule) != "" {
		wrapper.Respond(c, wrapper.ResponseOption[EVCorrectionResult]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}
	floor := trimOptional(body.Location, maxStampLocationLen)
	if body.Location != nil && floor == nil && strings.TrimSpace(*body.Location) != "" {
		wrapper.Respond(c, wrapper.ResponseOption[EVCorrectionResult]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	signagePhotos, ok := parsePrivilegeSignagePhotos(body.SignagePhotos)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[EVCorrectionResult]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	if !h.ensureProfile(c, claims, "ev correction") {
		return
	}

	updated, firstCorrection, err := h.repo.UpdateEVCharger(c.Request.Context(), id, UpdateEVInput{
		ProviderName:  providerName,
		Floor:         floor,
		Conditions:    rule,
		Connectors:    connectors,
		ChangedBy:     claims.Sub,
		SignagePhotos: signagePhotos,
	})
	if err != nil {
		slog.Error("update ev charger failed", "ev_charger_id", id, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[EVCorrectionResult]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}
	if updated == nil {
		wrapper.Respond(c, wrapper.ResponseOption[EVCorrectionResult]{
			HTTPStatus: http.StatusNotFound,
			Code:       app.CodeNotFound,
			Message:    app.MessageNotFound,
		})
		return
	}

	pointsAwarded := h.awardCorrectionPoints(c, claims.Sub, firstCorrection, "ev_charger", id, updated.PlaceID, "ev correction")

	wrapper.Respond(c, wrapper.ResponseOption[EVCorrectionResult]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data: &EVCorrectionResult{
			EVCharger:     *updated,
			PointsAwarded: pointsAwarded,
		},
	})
}

func normalizeEVProviderName(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if label, ok := evProviderLabels[strings.ToLower(trimmed)]; ok {
		return label
	}
	return trimmed
}

func expandEVConnectors(items []updateEVConnectorRequest) ([]EVConnectorDraft, bool) {
	if len(items) == 0 {
		return nil, false
	}
	out := make([]EVConnectorDraft, 0, len(items))
	for _, item := range items {
		mapped, ok := mapEVConnectorType(item.ConnectorType)
		if !ok {
			return nil, false
		}
		count := parsePositiveInt(item.Total, 0)
		if count < 1 {
			return nil, false
		}
		for i := 0; i < count; i++ {
			out = append(out, mapped)
			if len(out) > maxEVConnectors {
				return nil, false
			}
		}
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

func mapEVConnectorType(raw string) (EVConnectorDraft, bool) {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "TYPE_1", "TYPE_2":
		return EVConnectorDraft{ConnectorType: "AC_Type_2", PowerType: "AC", PowerKW: 7}, true
	case "TESLA":
		return EVConnectorDraft{ConnectorType: "Tesla", PowerType: "DC", PowerKW: 150}, true
	case "CCS1", "CCS2":
		return EVConnectorDraft{ConnectorType: "CCS2", PowerType: "DC", PowerKW: 50}, true
	case "CHADEMO":
		return EVConnectorDraft{ConnectorType: "CHAdeMO", PowerType: "DC", PowerKW: 50}, true
	default:
		return EVConnectorDraft{}, false
	}
}

func parsePositiveInt(raw string, fallback int) int {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fallback
	}
	n := 0
	for _, r := range trimmed {
		if r < '0' || r > '9' {
			return fallback
		}
		n = n*10 + int(r-'0')
		if n > 20 {
			return 20
		}
	}
	if n <= 0 {
		return fallback
	}
	return n
}

var evProviderLabels = map[string]string{
	"ea_anywhere":   "EA Anywhere",
	"pea_volta":     "PEA VOLTA",
	"elex_egat":     "EleX by EGAT",
	"tesla":         "Tesla Supercharger",
	"ptt_evstation": "PTT EV Station PluZ",
	"onion":         "Onion EV Charging",
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

	exists, err := h.repo.PlaceExists(c.Request.Context(), placeID)
	if err != nil {
		slog.Error("check place for privilege create failed", "place_id", placeID, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[any]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}
	if !exists {
		wrapper.Respond(c, wrapper.ResponseOption[any]{
			HTTPStatus: http.StatusNotFound,
			Code:       app.CodeNotFound,
			Message:    app.MessageNotFound,
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

	if !h.ensureProfile(c, claims, "privilege create") {
		return
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
		if errors.Is(err, ErrPlaceNotFound) {
			wrapper.Respond(c, wrapper.ResponseOption[any]{
				HTTPStatus: http.StatusNotFound,
				Code:       app.CodeNotFound,
				Message:    app.MessageNotFound,
			})
			return
		}
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

func parsePrivilegeSignagePhotos(raw *[]string) (*[]string, bool) {
	if raw == nil {
		return nil, true
	}
	cleaned := make([]string, 0, len(*raw))
	for _, item := range *raw {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		cleaned = append(cleaned, trimmed)
	}
	if len(cleaned) > maxPrivilegeSignagePhotos {
		return nil, false
	}
	if !mediaurl.ValidMediaURLs(cleaned, mediaurl.MaxURLLen) {
		return nil, false
	}
	return &cleaned, true
}
