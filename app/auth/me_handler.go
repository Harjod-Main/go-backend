package auth

import (
	"net/http"

	"github.com/RinTanth/go-backend/app/auth/supabaseauth"
	"github.com/RinTanth/go-common/app"
	"github.com/RinTanth/go-common/wrapper"
	"github.com/gin-gonic/gin"
)

type MeResponse struct {
	UserID string `json:"userId"`
	Email  string `json:"email,omitempty"`
	Role   string `json:"role,omitempty"`
}

// Me returns the authenticated Supabase user from the verified JWT.
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

	wrapper.Respond(c, wrapper.ResponseOption[MeResponse]{
		HTTPStatus: http.StatusOK,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data: &MeResponse{
			UserID: claims.Sub,
			Email:  claims.Email,
			Role:   claims.Role,
		},
	})
}
