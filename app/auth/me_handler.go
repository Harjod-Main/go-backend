package auth

import (
	"log/slog"
	"net/http"

	"github.com/RinTanth/go-backend/app/auth/supabaseauth"
	"github.com/RinTanth/go-backend/app/profile"
	"github.com/RinTanth/go-common/app"
	"github.com/RinTanth/go-common/wrapper"
	"github.com/gin-gonic/gin"
)

type MeResponse struct {
	UserID      string  `json:"userId"`
	Email       string  `json:"email,omitempty"`
	Role        string  `json:"role,omitempty"`
	DisplayName string  `json:"displayName,omitempty"`
	Username    string  `json:"username,omitempty"`
	AvatarURL   *string `json:"avatarUrl,omitempty"`
}

// Me returns the authenticated Supabase user plus profile fields.
func (h *Handler) Me(c *gin.Context) {
	claims, ok := supabaseauth.ClaimsFromGin(c)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[MeResponse]{
			HTTPStatus: http.StatusUnauthorized,
			Code:       app.CodeUnauthorized,
			Message:    app.MessageUnauthorized,
		})
		return
	}

	resp := MeResponse{
		UserID: claims.Sub,
		Email:  claims.Email,
		Role:   claims.Role,
	}

	seed := profile.OAuthSeedFromMetadata(claims.Email, claims.UserMetadata)

	if h.profileRepo != nil {
		p, err := h.profileRepo.Ensure(c.Request.Context(), claims.Sub, claims.Email, seed)
		if err != nil {
			slog.Error("ensure profile on /me failed", "user_id", claims.Sub, "error", err)
		} else if p != nil {
			resp.DisplayName = p.DisplayName
			resp.Username = p.Username
			resp.AvatarURL = p.AvatarURL
		}
	}

	wrapper.Respond(c, wrapper.ResponseOption[MeResponse]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       &resp,
	})
}