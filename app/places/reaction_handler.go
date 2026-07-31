package places

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/RinTanth/go-backend/app/auth/supabaseauth"
	"github.com/RinTanth/go-common/app"
	"github.com/RinTanth/go-common/wrapper"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// GetReaction handles GET /api/v1/places/:placeId/reaction (auth optional).
func (h *Handler) GetReaction(c *gin.Context) {
	placeID := strings.TrimSpace(c.Param("placeId"))
	if _, err := uuid.Parse(placeID); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[PlaceReactionResponse]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	exists, err := h.repo.PlaceExists(c.Request.Context(), placeID)
	if err != nil {
		slog.Error("check place for reaction failed", "place_id", placeID, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[PlaceReactionResponse]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}
	if !exists {
		wrapper.Respond(c, wrapper.ResponseOption[PlaceReactionResponse]{
			HTTPStatus: http.StatusNotFound,
			Code:       app.CodeNotFound,
			Message:    app.MessageNotFound,
		})
		return
	}

	userID := ""
	if claims, ok := supabaseauth.ClaimsFromGin(c); ok {
		userID = claims.Sub
	}
	resp, err := h.repo.GetPlaceReaction(c.Request.Context(), placeID, userID)
	if err != nil {
		slog.Error("get place reaction failed", "place_id", placeID, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[PlaceReactionResponse]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}
	wrapper.Respond(c, wrapper.ResponseOption[PlaceReactionResponse]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       resp,
	})
}

// SetReaction handles PUT /api/v1/places/:placeId/reaction (auth required).
func (h *Handler) SetReaction(c *gin.Context) {
	claims, ok := supabaseauth.ClaimsFromGin(c)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[PlaceReactionResponse]{
			HTTPStatus: http.StatusUnauthorized,
			Code:       app.CodeUnauthorized,
			Message:    app.MessageUnauthorized,
		})
		return
	}

	placeID := strings.TrimSpace(c.Param("placeId"))
	if _, err := uuid.Parse(placeID); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[PlaceReactionResponse]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	var body PlaceReactionRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[PlaceReactionResponse]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}
	if body.Reaction != PlaceReactionLike && body.Reaction != PlaceReactionUnlike {
		wrapper.Respond(c, wrapper.ResponseOption[PlaceReactionResponse]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	exists, err := h.repo.PlaceExists(c.Request.Context(), placeID)
	if err != nil {
		slog.Error("check place for set reaction failed", "place_id", placeID, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[PlaceReactionResponse]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}
	if !exists {
		wrapper.Respond(c, wrapper.ResponseOption[PlaceReactionResponse]{
			HTTPStatus: http.StatusNotFound,
			Code:       app.CodeNotFound,
			Message:    app.MessageNotFound,
		})
		return
	}

	resp, err := h.repo.SetPlaceReaction(c.Request.Context(), placeID, claims.Sub, body.Reaction)
	if err != nil {
		slog.Error("set place reaction failed", "place_id", placeID, "user_id", claims.Sub, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[PlaceReactionResponse]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}
	wrapper.Respond(c, wrapper.ResponseOption[PlaceReactionResponse]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       resp,
	})
}

// ClearReaction handles DELETE /api/v1/places/:placeId/reaction (auth required).
// If the current reaction matches ?reaction=, clears it; otherwise no-op clear of any reaction.
func (h *Handler) ClearReaction(c *gin.Context) {
	claims, ok := supabaseauth.ClaimsFromGin(c)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[PlaceReactionResponse]{
			HTTPStatus: http.StatusUnauthorized,
			Code:       app.CodeUnauthorized,
			Message:    app.MessageUnauthorized,
		})
		return
	}

	placeID := strings.TrimSpace(c.Param("placeId"))
	if _, err := uuid.Parse(placeID); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[PlaceReactionResponse]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	exists, err := h.repo.PlaceExists(c.Request.Context(), placeID)
	if err != nil {
		slog.Error("check place for clear reaction failed", "place_id", placeID, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[PlaceReactionResponse]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}
	if !exists {
		wrapper.Respond(c, wrapper.ResponseOption[PlaceReactionResponse]{
			HTTPStatus: http.StatusNotFound,
			Code:       app.CodeNotFound,
			Message:    app.MessageNotFound,
		})
		return
	}

	resp, err := h.repo.ClearPlaceReaction(c.Request.Context(), placeID, claims.Sub)
	if err != nil {
		slog.Error("clear place reaction failed", "place_id", placeID, "user_id", claims.Sub, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[PlaceReactionResponse]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}
	wrapper.Respond(c, wrapper.ResponseOption[PlaceReactionResponse]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       resp,
	})
}
