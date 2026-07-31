package submissions

import (
	"bytes"
	"log/slog"
	"net/http"
	"strings"

	"github.com/RinTanth/go-backend/app/auth/supabaseauth"
	"github.com/RinTanth/go-backend/app/mediaurl"
	"github.com/RinTanth/go-backend/app/points"
	"github.com/RinTanth/go-backend/app/profile"
	"github.com/RinTanth/go-common/app"
	"github.com/RinTanth/go-common/wrapper"
	"github.com/gin-gonic/gin"
)

const (
	maxSubmissionBodyBytes       = 256 * 1024
	maxSubmissionNameLen         = 160
	maxSubmissionAddressLen      = 500
	maxSubmissionPlaceTypeLen    = 80
	maxSubmissionMoneyFieldLen   = 80
	maxSubmissionAmenities       = 32
	maxSubmissionPhotos          = 10
	maxSubmissionSpecials        = 32
	maxSubmissionStringItemLen   = 200
	maxSubmissionJSONSectionSize = 32 * 1024
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

// Create handles POST /api/v1/places/submissions (auth required).
func (h *Handler) Create(c *gin.Context) {
	claims, ok := supabaseauth.ClaimsFromGin(c)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[Submission]{
			HTTPStatus: http.StatusUnauthorized,
			Code:       app.CodeUnauthorized,
			Message:    app.MessageUnauthorized,
		})
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSubmissionBodyBytes)
	var body CreateSubmissionRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[Submission]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	name := strings.TrimSpace(body.Name)
	if name == "" ||
		len(name) > maxSubmissionNameLen ||
		body.Latitude < -90 || body.Latitude > 90 ||
		body.Longitude < -180 || body.Longitude > 180 {
		wrapper.Respond(c, wrapper.ResponseOption[Submission]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}
	if len(body.PhotoURLs) > maxSubmissionPhotos ||
		len(body.RatePhotoURLs) > maxSubmissionPhotos ||
		len(body.Amenities) > maxSubmissionAmenities ||
		len(body.SpecialConditions) > maxSubmissionSpecials {
		wrapper.Respond(c, wrapper.ResponseOption[Submission]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}
	if body.FreeMinutes != nil && (*body.FreeMinutes < 0 || *body.FreeMinutes > 24*60) {
		wrapper.Respond(c, wrapper.ResponseOption[Submission]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}
	if !validStringItems(body.Amenities, maxSubmissionStringItemLen) ||
		!mediaurl.ValidMediaURLs(body.PhotoURLs, mediaurl.MaxURLLen) ||
		!mediaurl.ValidMediaURLs(body.RatePhotoURLs, mediaurl.MaxURLLen) ||
		!validStringItems(body.SpecialConditions, maxSubmissionStringItemLen) ||
		!validRawJSONSize(body.OpeningHours) ||
		!validRawJSONSize(body.RateTiers) ||
		!validRawJSONSize(body.ParkingStamps) ||
		!validRawJSONSize(body.ParkingReserved) ||
		!validRawJSONSize(body.ParkingEvCharges) {
		wrapper.Respond(c, wrapper.ResponseOption[Submission]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	var address *string
	if body.Address != nil {
		trimmed := strings.TrimSpace(*body.Address)
		if len(trimmed) > maxSubmissionAddressLen {
			wrapper.Respond(c, wrapper.ResponseOption[Submission]{
				HTTPStatus: http.StatusBadRequest,
				Code:       app.CodeBadRequest,
				Message:    app.MessageBadRequest,
			})
			return
		}
		if trimmed != "" {
			address = &trimmed
		}
	}
	var placeType *string
	if body.PlaceType != nil {
		trimmed := strings.TrimSpace(*body.PlaceType)
		if len(trimmed) > maxSubmissionPlaceTypeLen {
			wrapper.Respond(c, wrapper.ResponseOption[Submission]{
				HTTPStatus: http.StatusBadRequest,
				Code:       app.CodeBadRequest,
				Message:    app.MessageBadRequest,
			})
			return
		}
		if trimmed != "" {
			placeType = &trimmed
		}
	}
	if !validOptionalTrimmed(body.LostTicketFee, maxSubmissionMoneyFieldLen) ||
		!validOptionalTrimmed(body.OvernightFee, maxSubmissionMoneyFieldLen) {
		wrapper.Respond(c, wrapper.ResponseOption[Submission]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	uid := claims.Sub
	submission := Submission{
		UserID:            &uid,
		Name:              name,
		Address:           address,
		Latitude:          body.Latitude,
		Longitude:         body.Longitude,
		PlaceType:         placeType,
		Amenities:         body.Amenities,
		PhotoURLs:         body.PhotoURLs,
		RatePhotoURLs:     body.RatePhotoURLs,
		LostTicketFee:     body.LostTicketFee,
		OvernightFee:      body.OvernightFee,
		FreeMinutes:       body.FreeMinutes,
		OpeningHours:      body.OpeningHours,
		RateTiers:         body.RateTiers,
		SpecialConditions: body.SpecialConditions,
		ParkingStamps:     body.ParkingStamps,
		ParkingReserved:   body.ParkingReserved,
		ParkingEvCharges:  body.ParkingEvCharges,
	}

	if err := h.repo.Create(c.Request.Context(), &submission); err != nil {
		slog.Error("create place submission failed", "user_id", uid, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[Submission]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}

	if h.profiles != nil {
		if _, err := h.profiles.AddCreditPoints(c.Request.Context(), uid, points.PlaceSubmission); err != nil {
			slog.Error("award submission points failed", "user_id", uid, "error", err)
		}
	}

	wrapper.Respond(c, wrapper.ResponseOption[Submission]{
		HTTPStatus: http.StatusCreated,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       &submission,
	})
}

func validStringItems(items []string, maxLen int) bool {
	for _, item := range items {
		if trimmed := strings.TrimSpace(item); trimmed == "" || len(trimmed) > maxLen {
			return false
		}
	}
	return true
}

func validOptionalTrimmed(value *string, maxLen int) bool {
	if value == nil {
		return true
	}
	return len(strings.TrimSpace(*value)) <= maxLen
}

func validRawJSONSize(raw []byte) bool {
	if len(raw) == 0 {
		return true
	}
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) <= maxSubmissionJSONSectionSize
}
