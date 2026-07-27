package places

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/RinTanth/go-common/app"
	"github.com/RinTanth/go-common/wrapper"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

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
