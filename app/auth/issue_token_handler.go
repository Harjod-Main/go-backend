package auth

import (
	"log/slog"
	"net/http"

	"github.com/RinTanth/go-common/app"
	"github.com/RinTanth/go-common/token"
	"github.com/RinTanth/go-common/wrapper"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type IssueTokenRequest struct {
	UserId string `json:"userId" binding:"required"`
}

type IssueTokenResponse struct {
	AccessToken string `json:"accessToken"`
}

func (h *Handler) IssueToken(c *gin.Context) {
	var req IssueTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[IssueTokenResponse]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
			Err:        err,
		})
		return
	}

	claims := token.Claims{
		Sub: req.UserId,
		Jti: uuid.New().String(),
	}

	accessToken, err := h.token.SignES256(claims)
	if err != nil {
		slog.Error("failed to sign token", slog.String("err", err.Error()), slog.String("tag", "issue token"))
		wrapper.Respond(c, wrapper.ResponseOption[IssueTokenResponse]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
			Err:        err,
		})
		return
	}

	wrapper.Respond(c, wrapper.ResponseOption[IssueTokenResponse]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data: &IssueTokenResponse{
			AccessToken: accessToken,
		},
	})
}
