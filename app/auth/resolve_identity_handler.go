package auth

import (
	"log/slog"
	"net/http"

	"github.com/RinTanth/go-backend/app/auth/access"
	"github.com/RinTanth/go-common/app"
	"github.com/RinTanth/go-common/wrapper"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ResolveIdentityRequest struct {
	GoogleAccessToken string `json:"googleAccessToken" binding:"required"`
}

type ResolveIdentityResponse struct {
	IsUser       bool             `json:"isUser"`
	UserID       uuid.UUID        `json:"userId,omitempty"`
	Username     string           `json:"username,omitempty"`
	Email        string           `json:"email"`
	HashedEmail  string           `json:"hashedEmail,omitempty"`
	Role         *access.UserRole `json:"role,omitempty"`
	Country      *string          `json:"country,omitempty"`
	ProfileImage string           `json:"profileImage,omitempty"`
}

func (h *Handler) ResolveIdentify(c *gin.Context) {

	ctx := c.Request.Context()

	var req ResolveIdentityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[ResolveIdentityResponse]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
			Err:        err,
		})
		return
	}

	validateTokenResp, err := h.googleClient.ValidateAccessToken(ctx, req.GoogleAccessToken)
	if err != nil {
		slog.Error("fail to validate access token with client error", slog.String("err", err.Error()), slog.String("tag", "resolve identify"))
		wrapper.Respond(c, wrapper.ResponseOption[ResolveIdentityResponse]{
			HTTPStatus: http.StatusUnauthorized,
			Code:       app.CodeUnauthorized,
			Message:    "Invalid or expired Google access token",
			Err:        err,
		})
		return
	}

	if validateTokenResp.Code >= 400 {
		slog.Error("fail to validate access token with http error", slog.Int("httpCode", validateTokenResp.Code), slog.String("tag", "resolve identify"))
		wrapper.Respond(c, wrapper.ResponseOption[ResolveIdentityResponse]{
			HTTPStatus: http.StatusUnauthorized,
			Code:       app.CodeUnauthorized,
			Message:    "Invalid or expired Google access token",
		})
		return
	}

	userProfileResp, err := h.googleClient.GetUserProfile(ctx, req.GoogleAccessToken)
	if err != nil {
		slog.Error("fail to get user profile with client error", slog.String("err", err.Error()), slog.String("tag", "resolve identify"))
		wrapper.Respond(c, wrapper.ResponseOption[ResolveIdentityResponse]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
			Err:        err,
		})
		return
	}

	if userProfileResp.Code >= 400 {
		slog.Error("fail to get user profile with http error", slog.Int("httpCode", userProfileResp.Code), slog.String("tag", "resolve identify"))
		wrapper.Respond(c, wrapper.ResponseOption[ResolveIdentityResponse]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
		return
	}

	revokeGoogleTokenResponse, err := h.googleClient.RevokeToken(ctx, req.GoogleAccessToken)
	if err != nil {
		slog.Warn("fail to revoke google access token with client error", slog.String("err", err.Error()), slog.String("tag", "resolve identify"))
	}

	if revokeGoogleTokenResponse.Code >= 400 {
		slog.Warn("fail to revoke google access token with http error", slog.Int("httpCode", revokeGoogleTokenResponse.Code), slog.String("tag", "resolve identify"))
	}

	hashedEmail := h.hash.HashSha256EncodePepper(validateTokenResp.Response.Email)

	userInfo, err := h.pg.GetUserByEmail(ctx, hashedEmail)
	if err != nil {
		if err.Error() == "user not found" {
			slog.Info("user not found, returning Google profile data", slog.String("email", validateTokenResp.Response.Email), slog.String("tag", "resolve identify"))
			wrapper.Respond(c, wrapper.ResponseOption[ResolveIdentityResponse]{
				HTTPStatus: http.StatusOK,
				Code:       app.CodeSuccess,
				Message:    app.MessageSuccess,
				Data: &ResolveIdentityResponse{
					IsUser:       false,
					Email:        validateTokenResp.Response.Email,
					Username:     userProfileResp.Response.Name,
					ProfileImage: userProfileResp.Response.Picture,
				},
			})
			return
		}

		slog.Error("fail to get user by email", slog.String("err", err.Error()), slog.String("tag", "resolve identify"))
		wrapper.Respond(c, wrapper.ResponseOption[ResolveIdentityResponse]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
			Err:        err,
		})
		return
	}

	email, err := h.aesgcm.Decrypt(userInfo.Email)
	if err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[ResolveIdentityResponse]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
			Err:        err,
		})
		return
	}

	wrapper.Respond(c, wrapper.ResponseOption[ResolveIdentityResponse]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data: &ResolveIdentityResponse{
			IsUser:       true,
			UserID:       userInfo.GetID(),
			Username:     userInfo.Username,
			Email:        email,
			HashedEmail:  userInfo.EmailHashed,
			Role:         userInfo.Role,
			Country:      userInfo.Country,
			ProfileImage: userProfileResp.Response.Picture,
		},
	})
}
