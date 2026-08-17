package referrals_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/RinTanth/go-backend/app/auth/supabaseauth"
	"github.com/RinTanth/go-backend/app/referrals"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type stubRepo struct {
	in      referrals.AcceptInput
	outcome *referrals.AcceptOutcome
	err     error
	called  bool
}

func (s *stubRepo) Accept(_ context.Context, in referrals.AcceptInput) (*referrals.AcceptOutcome, error) {
	s.called = true
	s.in = in
	return s.outcome, s.err
}

func performAccept(t *testing.T, repo *stubRepo, body any, withClaims bool) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	handler := referrals.NewHandler(referrals.HandlerConfig{Repo: repo})
	engine := gin.New()
	engine.POST("/api/v1/referrals", func(c *gin.Context) {
		if withClaims {
			c.Set(supabaseauth.CtxClaimsKey, &supabaseauth.Claims{
				Sub:   "11111111-1111-1111-1111-111111111111",
				Email: "new@example.com",
			})
		}
		handler.Accept(c)
	})

	var payload []byte
	var err error
	switch v := body.(type) {
	case string:
		payload = []byte(v)
	default:
		payload, err = json.Marshal(v)
		require.NoError(t, err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/referrals", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func TestAccept_Created(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{
		outcome: &referrals.AcceptOutcome{
			Created:             true,
			ReferrerUserID:      "22222222-2222-2222-2222-222222222222",
			ReferrerUsername:    "alice",
			ReferrerDisplayName: "Alice",
			RefereePoints:       50,
			ReferrerPoints:      50,
		},
	}
	w := performAccept(t, repo, map[string]any{"inviteUsername": "alice"}, true)
	r.Equal(http.StatusCreated, w.Code)
	r.True(repo.called)
	r.Equal("alice", repo.in.InviteUsername)
	r.Equal("11111111-1111-1111-1111-111111111111", repo.in.RefereeUserID)

	var body struct {
		Data referrals.AcceptResponse `json:"data"`
	}
	r.NoError(json.Unmarshal(w.Body.Bytes(), &body))
	r.True(body.Data.Accepted)
	r.False(body.Data.AlreadyAccepted)
	r.Equal("alice", body.Data.ReferrerUsername)
	r.Equal(50, body.Data.RefereePoints)
}

func TestAccept_AlreadyAccepted(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{
		outcome: &referrals.AcceptOutcome{
			Created:             false,
			ReferrerUsername:    "alice",
			ReferrerDisplayName: "Alice",
			RefereePoints:       50,
			ReferrerPoints:      50,
		},
	}
	w := performAccept(t, repo, map[string]any{"inviteUsername": "alice"}, true)
	r.Equal(http.StatusOK, w.Code)

	var body struct {
		Data referrals.AcceptResponse `json:"data"`
	}
	r.NoError(json.Unmarshal(w.Body.Bytes(), &body))
	r.False(body.Data.Accepted)
	r.True(body.Data.AlreadyAccepted)
}

func TestAccept_MapsDomainErrors(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
	}{
		{"self", referrals.ErrSelfReferral, http.StatusBadRequest},
		{"invalid", referrals.ErrInvalidUsername, http.StatusBadRequest},
		{"missing referrer", referrals.ErrReferrerNotFound, http.StatusNotFound},
		{"already referred", referrals.ErrAlreadyReferred, http.StatusConflict},
		{"too old", referrals.ErrNotEligible, http.StatusConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)
			w := performAccept(t, &stubRepo{err: tc.err}, map[string]any{"inviteUsername": "alice"}, true)
			r.Equal(tc.status, w.Code)
		})
	}
}

func TestAccept_Unauthorized(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{}
	w := performAccept(t, repo, map[string]any{"inviteUsername": "alice"}, false)
	r.Equal(http.StatusUnauthorized, w.Code)
	r.False(repo.called)
}

func TestAccept_RejectsOversizedBody(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{}
	w := performAccept(t, repo, `{"inviteUsername":"`+strings.Repeat("a", 5000)+`"}`, true)
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(repo.called)
}

func TestAccept_RejectsInvalidJSON(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{}
	w := performAccept(t, repo, "{not-json", true)
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(repo.called)
}
