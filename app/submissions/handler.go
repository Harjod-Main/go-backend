package submissions

import (
	"bytes"
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
)

const (
	maxSubmissionBodyBytes       = 256 * 1024
	maxSubmissionNameLen         = 160
	maxSubmissionAddressLen      = 500
	maxSubmissionAdminAreaLen    = 120
	maxSubmissionPostalCodeLen   = 20
	maxSubmissionPlaceTypeLen    = 80
	maxSubmissionGooglePlaceIDLen = 256
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
	NotificationsSender *notifications.Sender
}

type Handler struct {
	repo     Repository
	profiles profile.Repository
	notificationsSender *notifications.Sender
}

func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{
		repo:                 cfg.Repo,
		profiles:            cfg.Profiles,
		notificationsSender: cfg.NotificationsSender,
	}
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
	nameTh := optionalTrimmed(body.NameTh, maxSubmissionNameLen)
	nameEn := optionalTrimmed(body.NameEn, maxSubmissionNameLen)
	if body.NameTh != nil && nameTh == nil && strings.TrimSpace(*body.NameTh) != "" {
		wrapper.Respond(c, wrapper.ResponseOption[Submission]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}
	if body.NameEn != nil && nameEn == nil && strings.TrimSpace(*body.NameEn) != "" {
		wrapper.Respond(c, wrapper.ResponseOption[Submission]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}
	if nameTh == nil {
		nameTh = &name
	}
	if nameEn == nil {
		nameEn = &name
	}
	googlePlaceID := optionalTrimmed(body.GooglePlaceID, maxSubmissionGooglePlaceIDLen)
	if body.GooglePlaceID != nil && googlePlaceID == nil && strings.TrimSpace(*body.GooglePlaceID) != "" {
		wrapper.Respond(c, wrapper.ResponseOption[Submission]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	addressTh, addressEn, subdistrictTh, subdistrictEn, districtTh, districtEn, provinceTh, provinceEn, postalCode, ok :=
		parseStructuredAddressFields(body)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[Submission]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}
	if addressTh == nil && address != nil {
		addressTh = address
	}
	if addressEn == nil && address != nil {
		addressEn = address
	}

	submission := Submission{
		UserID:            &uid,
		Name:              name,
		NameTh:            nameTh,
		NameEn:            nameEn,
		GooglePlaceID:     googlePlaceID,
		Address:           address,
		AddressTh:         addressTh,
		AddressEn:         addressEn,
		SubdistrictTh:     subdistrictTh,
		SubdistrictEn:     subdistrictEn,
		DistrictTh:        districtTh,
		DistrictEn:        districtEn,
		ProvinceTh:        provinceTh,
		ProvinceEn:        provinceEn,
		PostalCode:        postalCode,
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
		placeID := ""
		if submission.PlaceID != nil {
			placeID = *submission.PlaceID
		}
		if _, err := h.profiles.AddCreditPoints(c.Request.Context(), uid, profile.CreditAward{
			Amount:     points.PlaceSubmission,
			Reason:     points.ReasonPlaceSubmission,
			SourceType: "place_submission",
			SourceID:   submission.SubmissionID,
			PlaceID:    profile.OptionalPlaceID(placeID),
		}); err != nil {
			slog.Error("award submission points failed", "user_id", uid, "error", err)
		}
	}

	if h.notificationsSender != nil {
		_ = h.notificationsSender.SendToUser(
			c.Request.Context(),
			uid,
			notifications.NotificationEvent{
				Type:          "submission",
				PlaceID:       "",
				Title:         "Submission received",
				Body:          fmt.Sprintf("You earned +%d points for your submission.", points.PlaceSubmission),
				URL:           "/map",
				PointsAwarded: points.PlaceSubmission,
			},
		)
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

func optionalTrimmed(value *string, maxLen int) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" || len(trimmed) > maxLen {
		return nil
	}
	return &trimmed
}

// optionalTrimmedOrReject returns (nil, true) for empty/nil, (value, true) when
// valid, and (nil, false) when the non-empty value exceeds maxLen.
func optionalTrimmedOrReject(value *string, maxLen int) (*string, bool) {
	if value == nil {
		return nil, true
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, true
	}
	if len(trimmed) > maxLen {
		return nil, false
	}
	return &trimmed, true
}

func parseStructuredAddressFields(body CreateSubmissionRequest) (
	addressTh, addressEn, subdistrictTh, subdistrictEn, districtTh, districtEn, provinceTh, provinceEn, postalCode *string,
	ok bool,
) {
	type field struct {
		src *string
		max int
		dst **string
	}
	fields := []field{
		{body.AddressTh, maxSubmissionAddressLen, &addressTh},
		{body.AddressEn, maxSubmissionAddressLen, &addressEn},
		{body.SubdistrictTh, maxSubmissionAdminAreaLen, &subdistrictTh},
		{body.SubdistrictEn, maxSubmissionAdminAreaLen, &subdistrictEn},
		{body.DistrictTh, maxSubmissionAdminAreaLen, &districtTh},
		{body.DistrictEn, maxSubmissionAdminAreaLen, &districtEn},
		{body.ProvinceTh, maxSubmissionAdminAreaLen, &provinceTh},
		{body.ProvinceEn, maxSubmissionAdminAreaLen, &provinceEn},
		{body.PostalCode, maxSubmissionPostalCodeLen, &postalCode},
	}
	for _, f := range fields {
		v, fieldOK := optionalTrimmedOrReject(f.src, f.max)
		if !fieldOK {
			return nil, nil, nil, nil, nil, nil, nil, nil, nil, false
		}
		*f.dst = v
	}
	return addressTh, addressEn, subdistrictTh, subdistrictEn, districtTh, districtEn, provinceTh, provinceEn, postalCode, true
}

func validRawJSONSize(raw []byte) bool {
	if len(raw) == 0 {
		return true
	}
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) <= maxSubmissionJSONSectionSize
}
