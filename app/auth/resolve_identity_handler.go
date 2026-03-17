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
	IsMember       bool                              `json:"isMember"`
	MemberID       uuid.UUID                         `json:"memberId,omitempty"`
	Username       string                            `json:"username,omitempty"`
	Email          string                            `json:"email"`
	HashedEmail    string                            `json:"hashedEmail,omitempty"`
	Status         access.MemberStatusType           `json:"status,omitempty"`
	OrganizationID uuid.UUID                         `json:"organizationID,omitempty"`
	Role           access.OrganizationMemberRoleType `json:"role,omitempty"`
	ProfileImage   string                            `json:"profileImage,omitempty"`
}

func (h *handler) ResolveIdentify(c *gin.Context) {

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
	_ = hashedEmail

	// memberInfo, err := h.memberStorage.GetMemberByEmail(ctx, hashedEmail)
	// if err != nil {
	// 	if err.Error() == "member not found" {
	// 		// Member not found - this is a new user
	// 		slog.Info("member not found, returning Google profile data", slog.String("email", validateTokenResp.Response.Email), slog.String("tag", "resolve identify"))
	// 		wrapper.Respond(c, wrapper.ResponseOption[ResolveIdentityResponse]{
	// 			HTTPStatus: http.StatusOK,
	// 			Code:       app.CodeSuccess,
	// 			Message:    app.MessageSuccess,
	// 			Data: &ResolveIdentityResponse{
	// 				IsMember:     false,
	// 				Email:        validateTokenResp.Response.Email,
	// 				Username:     userProfileResp.Response.Name,
	// 				ProfileImage: userProfileResp.Response.Picture,
	// 			},
	// 		})
	// 		return
	// 	}

	// 	// Other errors - return internal error
	// 	slog.Error("fail to get member by email", slog.String("err", err.Error()), slog.String("tag", "resolve identify"))
	// 	wrapper.Respond(c, wrapper.ResponseOption[ResolveIdentityResponse]{
	// 		HTTPStatus: http.StatusInternalServerError,
	// 		Code:       app.CodeInternalError,
	// 		Message:    app.MessageInternalError,
	// 		Err:        err,
	// 	})
	// 	return
	// }

	// // Member exists - get organization info
	// organizationMember, err := h.organizationStorage.GetMemberOrganization(ctx, memberInfo.GetID())
	// if err != nil {
	// 	slog.Error("fail to get organization member", slog.String("err", err.Error()), slog.String("tag", "resolve identify"))
	// 	wrapper.Respond(c, wrapper.ResponseOption[ResolveIdentityResponse]{
	// 		HTTPStatus: http.StatusInternalServerError,
	// 		Code:       app.CodeInternalError,
	// 		Message:    app.MessageInternalError,
	// 		Err:        err,
	// 	})
	// 	return
	// }

	// email, err := h.aesgcm.Decrypt(memberInfo.EncryptedEmail)
	// if err != nil {
	// 	wrapper.Respond(c, wrapper.ResponseOption[ResolveIdentityResponse]{
	// 		HTTPStatus: http.StatusInternalServerError,
	// 		Code:       app.CodeInternalError,
	// 		Message:    app.MessageInternalError,
	// 		Err:        err,
	// 	})
	// 	return
	// }

	wrapper.Respond(c, wrapper.ResponseOption[ResolveIdentityResponse]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data: &ResolveIdentityResponse{
			IsMember: true,
			// MemberID:       memberInfo.GetID(),
			// Username:       memberInfo.Username,
			// Email:          email,
			// HashedEmail:    memberInfo.HashedEmail,
			// Status:         memberInfo.Status,
			// OrganizationID: organizationMember.GetOrganizationID(),
			// Role:           organizationMember.Role,
			ProfileImage: userProfileResp.Response.Picture,
		},
	})
}
