package submissions

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/RinTanth/go-backend/app/auth/supabaseauth"
	"github.com/RinTanth/go-common/app"
	"github.com/RinTanth/go-common/wrapper"
	"github.com/gin-gonic/gin"
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
	if name == "" || body.Latitude < -90 || body.Latitude > 90 || body.Longitude < -180 || body.Longitude > 180 {
		wrapper.Respond(c, wrapper.ResponseOption[Submission]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}
	if len(body.PhotoURLs) > 10 || len(body.RatePhotoURLs) > 10 {
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

	var address *string
	if body.Address != nil {
		trimmed := strings.TrimSpace(*body.Address)
		if trimmed != "" {
			address = &trimmed
		}
	}
	var placeType *string
	if body.PlaceType != nil {
		trimmed := strings.TrimSpace(*body.PlaceType)
		if trimmed != "" {
			placeType = &trimmed
		}
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

	wrapper.Respond(c, wrapper.ResponseOption[Submission]{
		HTTPStatus: http.StatusCreated,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       &submission,
	})
}
