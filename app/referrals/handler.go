package referrals

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/RinTanth/go-backend/app/auth/supabaseauth"
	"github.com/RinTanth/go-backend/app/notifications"
	"github.com/RinTanth/go-backend/app/points"
	"github.com/RinTanth/go-common/app"
	"github.com/RinTanth/go-common/wrapper"
	"github.com/gin-gonic/gin"
)

const maxAcceptBodyBytes = 4 * 1024

type HandlerConfig struct {
	Repo                Repository
	NotificationsSender *notifications.Sender
}

type Handler struct {
	repo                Repository
	notificationsSender *notifications.Sender
}

func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{
		repo:                cfg.Repo,
		notificationsSender: cfg.NotificationsSender,
	}
}

// Accept handles POST /api/v1/referrals (auth required).
func (h *Handler) Accept(c *gin.Context) {
	claims, ok := supabaseauth.ClaimsFromGin(c)
	if !ok {
		wrapper.Respond(c, wrapper.ResponseOption[AcceptResponse]{
			HTTPStatus: http.StatusUnauthorized,
			Code:       app.CodeUnauthorized,
			Message:    app.MessageUnauthorized,
		})
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAcceptBodyBytes)

	var body AcceptRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[AcceptResponse]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    app.MessageBadRequest,
		})
		return
	}

	outcome, err := h.repo.Accept(c.Request.Context(), AcceptInput{
		RefereeUserID:  claims.Sub,
		InviteUsername: strings.TrimSpace(body.InviteUsername),
	})
	if err != nil {
		respondAcceptError(c, claims.Sub, err)
		return
	}

	if outcome.Created && h.notificationsSender != nil {
		_ = h.notificationsSender.SendToUser(
			c.Request.Context(),
			outcome.ReferrerUserID,
			notifications.NotificationEvent{
				Type:          "referral",
				Title:         "A friend joined Harjod",
				Body:          fmt.Sprintf("You earned +%d points for inviting a friend.", points.ReferralReferrer),
				URL:           "/setting/invite-friends",
				PointsAwarded: points.ReferralReferrer,
			},
		)
	}

	status := http.StatusOK
	if outcome.Created {
		status = http.StatusCreated
	}
	resp := AcceptResponse{
		Accepted:            outcome.Created,
		AlreadyAccepted:     !outcome.Created,
		ReferrerUsername:    outcome.ReferrerUsername,
		ReferrerDisplayName: outcome.ReferrerDisplayName,
		RefereePoints:       outcome.RefereePoints,
		ReferrerPoints:      outcome.ReferrerPoints,
	}
	wrapper.Respond(c, wrapper.ResponseOption[AcceptResponse]{
		HTTPStatus: status,
		Code:       app.CodeSuccess,
		Message:    app.MessageSuccess,
		Data:       &resp,
	})
}

func respondAcceptError(c *gin.Context, userID string, err error) {
	switch {
	case errors.Is(err, ErrInvalidUsername):
		wrapper.Respond(c, wrapper.ResponseOption[AcceptResponse]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    "invalid invite username",
		})
	case errors.Is(err, ErrSelfReferral):
		wrapper.Respond(c, wrapper.ResponseOption[AcceptResponse]{
			HTTPStatus: http.StatusBadRequest,
			Code:       app.CodeBadRequest,
			Message:    "cannot use your own invite",
		})
	case errors.Is(err, ErrReferrerNotFound), errors.Is(err, ErrRefereeNotFound):
		wrapper.Respond(c, wrapper.ResponseOption[AcceptResponse]{
			HTTPStatus: http.StatusNotFound,
			Code:       app.CodeNotFound,
			Message:    "referrer not found",
		})
	case errors.Is(err, ErrAlreadyReferred):
		wrapper.Respond(c, wrapper.ResponseOption[AcceptResponse]{
			HTTPStatus: http.StatusConflict,
			Code:       app.CodeBadRequest,
			Message:    "already referred",
		})
	case errors.Is(err, ErrNotEligible):
		wrapper.Respond(c, wrapper.ResponseOption[AcceptResponse]{
			HTTPStatus: http.StatusConflict,
			Code:       app.CodeBadRequest,
			Message:    "referral is only for new accounts",
		})
	default:
		slog.Error("accept referral failed", "user_id", userID, "error", err)
		wrapper.Respond(c, wrapper.ResponseOption[AcceptResponse]{
			HTTPStatus: http.StatusInternalServerError,
			Code:       app.CodeInternalError,
			Message:    app.MessageInternalError,
		})
	}
}
