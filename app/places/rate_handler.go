package places

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/RinTanth/go-backend/app/auth/supabaseauth"
	"github.com/RinTanth/go-backend/app/points"
	"github.com/RinTanth/go-backend/app/profile"
	"github.com/RinTanth/go-common/app"
	"github.com/RinTanth/go-common/wrapper"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	maxRateCorrectionBodyBytes = 32 * 1024
	maxRateNotesLen            = 4000
	maxRateSpecialEntryLen     = 3000
)

// GetRate returns the parking rate sheet for a place (public read).
func (h *Handler) GetRate(c *gin.Context) {
	placeID := strings.TrimSpace(c.Param("placeId"))
	if _, err := uuid.Parse(placeID); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[PlaceRateDetail]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	rate, err := h.repo.GetPlaceRate(c.Request.Context(), placeID)
	if err != nil {
		slog.Error("place rate failed", "place_id", placeID, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[PlaceRateDetail]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}

	wrapper.Respond(c, wrapper.ResponseOption[PlaceRateDetail]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       rate,
	})
}

type rateTierCorrectionRequest struct {
	FromHour     float64  `json:"fromHour"`
	ToHour       *float64 `json:"toHour"`
	PricePerHour float64  `json:"pricePerHour"`
	Unit         string   `json:"unit"`
}

type updateRateRequest struct {
	FreeMinutes       *int                        `json:"freeMinutes"`
	LostTicketFee     *float64                    `json:"lostTicketFee"`
	OvernightFee      *float64                    `json:"overnightFee"`
	SpecialConditions []string                    `json:"specialConditions"`
	RateTiers         []rateTierCorrectionRequest `json:"rateTiers"`
}

type RateTierUnit string

const (
	RateUnitHourly RateTierUnit = "hourly"
	RateUnitFlat   RateTierUnit = "flat"
)

func joinSpecialConditions(values []string) *string {
	parts := make([]string, 0, len(values))
	for _, v := range values {
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			continue
		}
		parts = append(parts, trimmed)
	}
	if len(parts) == 0 {
		return nil
	}
	joined := strings.Join(parts, "\n")
	if len(joined) > maxRateNotesLen {
		return nil
	}
	return &joined
}

// UpdateRate handles PATCH /api/v1/places/:placeId/rate (auth required).
func (h *Handler) UpdateRate(c *gin.Context) {
	claims, ok := supabaseauth.ClaimsFromGin(c)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[RateCorrectionResult]{
			HTTPStatus: http.StatusUnauthorized,
			Code:       app.CodeUnauthorized,
			Message:    app.MessageUnauthorized,
		})
		return
	}

	placeID := strings.TrimSpace(c.Param("placeId"))
	if _, err := uuid.Parse(placeID); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[RateCorrectionResult]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRateCorrectionBodyBytes)
	var body updateRateRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[RateCorrectionResult]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	if len(body.RateTiers) == 0 {
		wrapper.Respond(c, wrapper.ResponseOption[RateCorrectionResult]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	// Validate and map inputs.
	rateTiers := make([]RateTierDraft, 0, len(body.RateTiers))
	for _, tier := range body.RateTiers {
		unitKey := strings.ToLower(strings.TrimSpace(tier.Unit))
		switch RateTierUnit(unitKey) {
		case RateUnitHourly, RateUnitFlat:
		default:
			wrapper.Respond(c, wrapper.ResponseOption[RateCorrectionResult]{
				HTTPStatus: http.StatusBadRequest,
				Code:       app.CodeBadRequest,
				Message:    app.MessageBadRequest,
			})
			return
		}

		if tier.PricePerHour < 0 {
			wrapper.Respond(c, wrapper.ResponseOption[RateCorrectionResult]{
				HTTPStatus: http.StatusBadRequest,
				Code:       app.CodeBadRequest,
				Message:    app.MessageBadRequest,
			})
			return
		}

		if tier.ToHour != nil && *tier.ToHour <= tier.FromHour && unitKey == string(RateUnitHourly) {
			wrapper.Respond(c, wrapper.ResponseOption[RateCorrectionResult]{
				HTTPStatus: http.StatusBadRequest,
				Code:       app.CodeBadRequest,
				Message:    app.MessageBadRequest,
			})
			return
		}

		var toHour *float64
		if tier.ToHour != nil {
			v := *tier.ToHour
			toHour = &v
		}

		rateTiers = append(rateTiers, RateTierDraft{
			FromHour: tier.FromHour,
			ToHour:   toHour,
			Price:    tier.PricePerHour,
			Unit:     unitKey,
		})
	}

	// Notes come from special conditions.
	specialConditions := body.SpecialConditions
	for _, entry := range specialConditions {
		if l := len(strings.TrimSpace(entry)); l > maxRateSpecialEntryLen {
			wrapper.Respond(c, wrapper.ResponseOption[RateCorrectionResult]{
				HTTPStatus: http.StatusBadRequest,
				Code:       app.CodeBadRequest,
				Message:    app.MessageBadRequest,
			})
			return
		}
	}

	notes := joinSpecialConditions(specialConditions)

	if h.profiles != nil {
		seed := profile.OAuthSeedFromMetadata(claims.Email, claims.UserMetadata)
		if _, err := h.profiles.Ensure(c.Request.Context(), claims.Sub, claims.Email, seed); err != nil {
			slog.Error("ensure profile before rate correction failed", "user_id", claims.Sub, "error", err)
			wrapper.Respond(c, wrapper.ResponseOption[RateCorrectionResult]{
				HTTPStatus: http.StatusInternalServerError,
				Code:       app.CodeInternalError,
				Message:    app.MessageInternalError,
			})
			return
		}
	}

	updated, firstCorrection, err := h.repo.UpdateRate(c.Request.Context(), placeID, UpdateRateInput{
		FreeMinutes:   body.FreeMinutes,
		LostTicketFee: body.LostTicketFee,
		OvernightFee:  body.OvernightFee,
		Notes:         notes,
		RateTiers:     rateTiers,
		ChangedBy:     claims.Sub,
	})
	if err != nil {
		slog.Error("update rate failed", "place_id", placeID, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[RateCorrectionResult]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}
	if updated == nil {
		wrapper.Respond(c, wrapper.ResponseOption[RateCorrectionResult]{
			HTTPStatus: http.StatusNotFound,
			Code:       app.CodeNotFound,
			Message:    app.MessageNotFound,
		})
		return
	}

	h.invalidateMapList(c.Request.Context())

	pointsAwarded := 0
	if firstCorrection && h.profiles != nil {
		if _, err := h.profiles.AddCreditPoints(c.Request.Context(), claims.Sub, profile.CreditAward{
			Amount:     points.PrivilegeCorrection,
			Reason:     points.ReasonCorrection,
			SourceType: "rate",
			SourceID:   placeID,
			PlaceID:    profile.OptionalPlaceID(placeID),
		}); err != nil {
			slog.Error("award rate correction points failed", "user_id", claims.Sub, "error", err)
		} else {
			pointsAwarded = points.PrivilegeCorrection
		}
	}

	wrapper.Respond(c, wrapper.ResponseOption[RateCorrectionResult]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data: &RateCorrectionResult{
			Rate:          *updated,
			PointsAwarded: pointsAwarded,
		},
	})
}
