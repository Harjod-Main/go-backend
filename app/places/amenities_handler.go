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
	maxAmenityCorrectionBodyBytes = 8 * 1024
)

type updateParkingAmenitiesRequest struct {
	HasCover         bool    `json:"hasCover"`
	HasEvCharging    bool    `json:"hasEvCharging"`
	HasValet         bool    `json:"hasValet"`
	TotalSpaces      *int    `json:"totalSpaces"`
	TransitAccess    bool    `json:"transitAccess"`
	TransitAccessType *string `json:"transitAccessType"`
}

// UpdateParkingAmenities handles PATCH /api/v1/places/:placeId/amenities (auth required).
func (h *Handler) UpdateParkingAmenities(c *gin.Context) {
	claims, ok := supabaseauth.ClaimsFromGin(c)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[ParkingAmenitiesCorrectionResult]{
			HTTPStatus: http.StatusUnauthorized,
			Code:       app.CodeUnauthorized,
			Message:    app.MessageUnauthorized,
		})
		return
	}

	placeID := strings.TrimSpace(c.Param("placeId"))
	if _, err := uuid.Parse(placeID); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[ParkingAmenitiesCorrectionResult]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAmenityCorrectionBodyBytes)
	var body updateParkingAmenitiesRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[ParkingAmenitiesCorrectionResult]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	// Normalize optional fields.
	totalSpaces := body.TotalSpaces
	if totalSpaces != nil && *totalSpaces <= 0 {
		totalSpaces = nil
	}

	transitAccessType := body.TransitAccessType
	if !body.TransitAccess {
		transitAccessType = nil
	}
	if transitAccessType != nil {
		trimmed := strings.TrimSpace(*transitAccessType)
		if trimmed == "" {
			transitAccessType = nil
		} else {
			transitAccessType = &trimmed
		}
	}

	if h.profiles != nil {
		seed := profile.OAuthSeedFromMetadata(claims.Email, claims.UserMetadata)
		if _, err := h.profiles.Ensure(c.Request.Context(), claims.Sub, claims.Email, seed); err != nil {
			slog.Error("ensure profile before amenities correction failed", "user_id", claims.Sub, "error", err)
			wrapper.Respond(c, wrapper.ResponseOption[ParkingAmenitiesCorrectionResult]{
				HTTPStatus: http.StatusInternalServerError,
				Code:       app.CodeInternalError,
				Message:    app.MessageInternalError,
			})
			return
		}
	}

	updated, firstCorrection, err := h.repo.UpdateParkingAmenities(c.Request.Context(), placeID, UpdateParkingAmenitiesInput{
		HasCover:         body.HasCover,
		HasEvCharging:    body.HasEvCharging,
		HasValet:         body.HasValet,
		TotalSpaces:      totalSpaces,
		TransitAccess:    body.TransitAccess,
		TransitAccessType: transitAccessType,
		ChangedBy:       claims.Sub,
	})
	if err != nil {
		slog.Error("update parking amenities failed", "place_id", placeID, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[ParkingAmenitiesCorrectionResult]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}

	if updated == nil {
		wrapper.Respond(c, wrapper.ResponseOption[ParkingAmenitiesCorrectionResult]{
			HTTPStatus: http.StatusNotFound,
			Code:       app.CodeNotFound,
			Message:    app.MessageNotFound,
		})
		return
	}

	pointsAwarded := 0
	if firstCorrection && h.profiles != nil {
		if _, err := h.profiles.AddCreditPoints(c.Request.Context(), claims.Sub, points.PrivilegeCorrection); err != nil {
			slog.Error("award amenities correction points failed", "user_id", claims.Sub, "error", err)
		} else {
			pointsAwarded = points.PrivilegeCorrection
		}
	}

	// Note: we always return success because repo UpdateParkingAmenities already
	// performed DB work and this flow is user-facing. If you want strict 404
	// semantics, return a non-nil updated result from repo and check it here.
	wrapper.Respond(c, wrapper.ResponseOption[ParkingAmenitiesCorrectionResult]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data: &ParkingAmenitiesCorrectionResult{
			PointsAwarded: pointsAwarded,
		},
	})
}

