package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	aesgcm_mocks "github.com/RinTanth/go-common/aesgcm/mocks"
	"github.com/RinTanth/go-common/app"
	hash_mocks "github.com/RinTanth/go-common/hash/mocks"
	"github.com/RinTanth/go-common/httpclient"
	"github.com/RinTanth/go-common/wrapper"

	"github.com/RinTanth/go-backend/app/auth"
	"github.com/RinTanth/go-backend/app/auth/access"
	access_mocks "github.com/RinTanth/go-backend/app/auth/access/mocks"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveIdentity(t *testing.T) {

	r := require.New(t)

	memberID := uuid.New().String()
	organizationID := uuid.New().String()
	now := time.Date(2026, 1, 20, 10, 0, 0, 0, time.Local)

	type mockArgs struct {
		googleClient        *access_mocks.GoogleClientMock
		memberStorage       *access_mocks.MemberStorageMock
		organizationStorage *access_mocks.OrganizationStorageMock
		hash                *hash_mocks.HashManagerMock
		aesgcm              *aesgcm_mocks.AesgcmMock
	}

	type args struct {
		ctx context.Context
		req auth.ResolveIdentityRequest
	}

	type want struct {
		err     bool
		code    app.Code
		Message app.Message
		data    wrapper.ResponseOption[auth.ResolveIdentityResponse]
	}

	tests := []struct {
		name    string
		prepare func(m mockArgs, args args)
		args    args
		want    want
	}{
		{
			name: "success, case valid request",
			prepare: func(m mockArgs, args args) {
				m.googleClient.
					EXPECT().
					ValidateAccessToken(args.ctx, args.req.GoogleAccessToken).
					Return(httpclient.Response[access.GoogleTokenInfoResponse]{
						Code: http.StatusOK,
						Response: access.GoogleTokenInfoResponse{
							Email: "test@example.com",
						},
					}, nil)

				m.googleClient.
					EXPECT().
					GetUserProfile(args.ctx, args.req.GoogleAccessToken).
					Return(httpclient.Response[access.GoogleUserProfileResponse]{
						Code: http.StatusOK,
						Response: access.GoogleUserProfileResponse{
							Picture: "https://example.com/profile.jpg",
						},
					}, nil)

				m.googleClient.EXPECT().RevokeToken(args.ctx, args.req.GoogleAccessToken).
					Return(httpclient.Response[any]{
						Code: http.StatusOK,
					}, nil)

				m.hash.
					EXPECT().
					HashSha256EncodePepper("test@example.com").
					Return("hashed_email")

				m.memberStorage.
					EXPECT().
					GetMemberByEmail(args.ctx, "hashed_email").
					Return(access.Member{
						MemberID:       memberID,
						Username:       "testuser",
						EncryptedEmail: "encrypted_email",
						HashedEmail:    "hashed_email",
						Status:         access.MemberStatusActive,
						CreatedAt:      now,
						UpdatedAt:      now,
					}, nil)

				m.organizationStorage.
					EXPECT().
					GetMemberOrganization(args.ctx, uuid.MustParse(memberID)).
					Return(access.OrganizationMember{
						OrganizationID: organizationID,
						MemberID:       memberID,
						Role:           access.OrganizationMemberRoleAdmin,
						Status:         access.OrganizationMemberStatusActive,
						CreatedAt:      now,
						UpdatedAt:      now,
					}, nil)

				m.aesgcm.
					EXPECT().
					Decrypt("encrypted_email").
					Return("test@example.com", nil)
			},
			args: args{
				req: auth.ResolveIdentityRequest{
					GoogleAccessToken: "valid_google_access_token",
				},
			},
			want: want{
				err:     false,
				code:    app.CodeSuccess,
				Message: app.MessageSuccess,
				data: wrapper.ResponseOption[auth.ResolveIdentityResponse]{
					HTTPStatus: http.StatusOK,
					Code:       app.CodeSuccess,
					Message:    app.MessageSuccess,
					Data: &auth.ResolveIdentityResponse{
						IsMember:       true,
						MemberID:       uuid.MustParse(memberID),
						Username:       "testuser",
						Email:          "test@example.com",
						HashedEmail:    "hashed_email",
						Status:         access.MemberStatusActive,
						OrganizationID: uuid.MustParse(organizationID),
						Role:           access.OrganizationMemberRoleAdmin,
						ProfileImage:   "https://example.com/profile.jpg",
					},
				},
			},
		},
		{
			name: "success, case valid request but revoke google token error",
			prepare: func(m mockArgs, args args) {
				m.googleClient.
					EXPECT().
					ValidateAccessToken(args.ctx, args.req.GoogleAccessToken).
					Return(httpclient.Response[access.GoogleTokenInfoResponse]{
						Code: http.StatusOK,
						Response: access.GoogleTokenInfoResponse{
							Email: "test@example.com",
						},
					}, nil)

				m.googleClient.
					EXPECT().
					GetUserProfile(args.ctx, args.req.GoogleAccessToken).
					Return(httpclient.Response[access.GoogleUserProfileResponse]{
						Code: http.StatusOK,
						Response: access.GoogleUserProfileResponse{
							Picture: "https://example.com/profile.jpg",
						},
					}, nil)

				m.googleClient.EXPECT().RevokeToken(args.ctx, args.req.GoogleAccessToken).
					Return(httpclient.Response[any]{
						Code: http.StatusInternalServerError,
					}, errors.New("some-error"))

				m.hash.
					EXPECT().
					HashSha256EncodePepper("test@example.com").
					Return("hashed_email")

				m.memberStorage.
					EXPECT().
					GetMemberByEmail(args.ctx, "hashed_email").
					Return(access.Member{
						MemberID:       memberID,
						Username:       "testuser",
						EncryptedEmail: "encrypted_email",
						HashedEmail:    "hashed_email",
						Status:         access.MemberStatusActive,
						CreatedAt:      now,
						UpdatedAt:      now,
					}, nil)

				m.organizationStorage.
					EXPECT().
					GetMemberOrganization(args.ctx, uuid.MustParse(memberID)).
					Return(access.OrganizationMember{
						OrganizationID: organizationID,
						MemberID:       memberID,
						Role:           access.OrganizationMemberRoleAdmin,
						Status:         access.OrganizationMemberStatusActive,
						CreatedAt:      now,
						UpdatedAt:      now,
					}, nil)

				m.aesgcm.
					EXPECT().
					Decrypt("encrypted_email").
					Return("test@example.com", nil)
			},
			args: args{
				req: auth.ResolveIdentityRequest{
					GoogleAccessToken: "valid_google_access_token",
				},
			},
			want: want{
				err:     false,
				code:    app.CodeSuccess,
				Message: app.MessageSuccess,
				data: wrapper.ResponseOption[auth.ResolveIdentityResponse]{
					HTTPStatus: http.StatusOK,
					Code:       app.CodeSuccess,
					Message:    app.MessageSuccess,
					Data: &auth.ResolveIdentityResponse{
						IsMember:       true,
						MemberID:       uuid.MustParse(memberID),
						Username:       "testuser",
						Email:          "test@example.com",
						HashedEmail:    "hashed_email",
						Status:         access.MemberStatusActive,
						OrganizationID: uuid.MustParse(organizationID),
						Role:           access.OrganizationMemberRoleAdmin,
						ProfileImage:   "https://example.com/profile.jpg",
					},
				},
			},
		},
		{
			name: "success, case member not found (new user)",
			prepare: func(m mockArgs, args args) {
				m.googleClient.
					EXPECT().
					ValidateAccessToken(args.ctx, args.req.GoogleAccessToken).
					Return(httpclient.Response[access.GoogleTokenInfoResponse]{
						Code: http.StatusOK,
						Response: access.GoogleTokenInfoResponse{
							Email: "newuser@example.com",
						},
					}, nil)

				m.googleClient.
					EXPECT().
					GetUserProfile(args.ctx, args.req.GoogleAccessToken).
					Return(httpclient.Response[access.GoogleUserProfileResponse]{
						Code: http.StatusOK,
						Response: access.GoogleUserProfileResponse{
							Name:    "New User",
							Picture: "https://example.com/newuser.jpg",
						},
					}, nil)

				m.googleClient.EXPECT().RevokeToken(args.ctx, args.req.GoogleAccessToken).
					Return(httpclient.Response[any]{
						Code: http.StatusOK,
					}, nil)

				m.hash.
					EXPECT().
					HashSha256EncodePepper("newuser@example.com").
					Return("hashed_email")

				m.memberStorage.
					EXPECT().
					GetMemberByEmail(args.ctx, "hashed_email").
					Return(access.Member{}, errors.New("member not found"))
			},
			args: args{
				req: auth.ResolveIdentityRequest{
					GoogleAccessToken: "valid_google_access_token",
				},
			},
			want: want{
				err:     false,
				code:    app.CodeSuccess,
				Message: app.MessageSuccess,
				data: wrapper.ResponseOption[auth.ResolveIdentityResponse]{
					HTTPStatus: http.StatusOK,
					Code:       app.CodeSuccess,
					Message:    app.MessageSuccess,
					Data: &auth.ResolveIdentityResponse{
						IsMember:     false,
						Email:        "newuser@example.com",
						Username:     "New User",
						ProfileImage: "https://example.com/newuser.jpg",
					},
				},
			},
		},
		{
			name: "fail, case invalid request body - missing google access token",
			prepare: func(m mockArgs, args args) {
				// no calls
			},
			args: args{
				req: auth.ResolveIdentityRequest{
					// missing GoogleAccessToken
				},
			},
			want: want{
				err:     true,
				code:    app.CodeBadRequest,
				Message: app.MessageBadRequest,
				data: wrapper.ResponseOption[auth.ResolveIdentityResponse]{
					HTTPStatus: http.StatusBadRequest,
					Code:       app.CodeBadRequest,
					Message:    app.MessageBadRequest,
				},
			},
		},
		{
			name: "fail, case google client ValidateAccessToken error",
			prepare: func(m mockArgs, args args) {
				m.googleClient.
					EXPECT().
					ValidateAccessToken(args.ctx, args.req.GoogleAccessToken).
					Return(httpclient.Response[access.GoogleTokenInfoResponse]{}, assert.AnError)
			},
			args: args{
				req: auth.ResolveIdentityRequest{
					GoogleAccessToken: "valid_google_access_token",
				},
			},
			want: want{
				err:     true,
				code:    app.CodeUnauthorized,
				Message: "Invalid or expired Google access token",
				data: wrapper.ResponseOption[auth.ResolveIdentityResponse]{
					HTTPStatus: http.StatusUnauthorized,
					Code:       app.CodeUnauthorized,
					Message:    "Invalid or expired Google access token",
				},
			},
		},
		{
			name: "fail, case google client ValidateAccessToken http error code >= 400",
			prepare: func(m mockArgs, args args) {
				m.googleClient.
					EXPECT().
					ValidateAccessToken(args.ctx, args.req.GoogleAccessToken).
					Return(httpclient.Response[access.GoogleTokenInfoResponse]{
						Code: http.StatusUnauthorized,
						Response: access.GoogleTokenInfoResponse{
							Error: "invalid_token",
						},
					}, nil)
			},
			args: args{
				req: auth.ResolveIdentityRequest{
					GoogleAccessToken: "invalid_google_access_token",
				},
			},
			want: want{
				err:     true,
				code:    app.CodeUnauthorized,
				Message: "Invalid or expired Google access token",
				data: wrapper.ResponseOption[auth.ResolveIdentityResponse]{
					HTTPStatus: http.StatusUnauthorized,
					Code:       app.CodeUnauthorized,
					Message:    "Invalid or expired Google access token",
				},
			},
		},
		{
			name: "fail, case google client GetUserProfile error",
			prepare: func(m mockArgs, args args) {
				m.googleClient.
					EXPECT().
					ValidateAccessToken(args.ctx, args.req.GoogleAccessToken).
					Return(httpclient.Response[access.GoogleTokenInfoResponse]{
						Code: http.StatusOK,
						Response: access.GoogleTokenInfoResponse{
							Email: "test@example.com",
						},
					}, nil)

				m.googleClient.
					EXPECT().
					GetUserProfile(args.ctx, args.req.GoogleAccessToken).
					Return(httpclient.Response[access.GoogleUserProfileResponse]{}, assert.AnError)
			},
			args: args{
				req: auth.ResolveIdentityRequest{
					GoogleAccessToken: "valid_google_access_token",
				},
			},
			want: want{
				err:     true,
				code:    app.CodeInternalError,
				Message: app.MessageInternalError,
				data: wrapper.ResponseOption[auth.ResolveIdentityResponse]{
					HTTPStatus: http.StatusInternalServerError,
					Code:       app.CodeInternalError,
					Message:    app.MessageInternalError,
				},
			},
		},
		{
			name: "fail, case google client GetUserProfile http error code >= 400",
			prepare: func(m mockArgs, args args) {
				m.googleClient.
					EXPECT().
					ValidateAccessToken(args.ctx, args.req.GoogleAccessToken).
					Return(httpclient.Response[access.GoogleTokenInfoResponse]{
						Code: http.StatusOK,
						Response: access.GoogleTokenInfoResponse{
							Email: "test@example.com",
						},
					}, nil)

				m.googleClient.
					EXPECT().
					GetUserProfile(args.ctx, args.req.GoogleAccessToken).
					Return(httpclient.Response[access.GoogleUserProfileResponse]{
						Code: http.StatusUnauthorized,
					}, nil)
			},
			args: args{
				req: auth.ResolveIdentityRequest{
					GoogleAccessToken: "valid_google_access_token",
				},
			},
			want: want{
				err:     true,
				code:    app.CodeInternalError,
				Message: app.MessageInternalError,
				data: wrapper.ResponseOption[auth.ResolveIdentityResponse]{
					HTTPStatus: http.StatusInternalServerError,
					Code:       app.CodeInternalError,
					Message:    app.MessageInternalError,
				},
			},
		},
		{
			name: "fail, case member storage GetMemberByEmail error",
			prepare: func(m mockArgs, args args) {
				m.googleClient.
					EXPECT().
					ValidateAccessToken(args.ctx, args.req.GoogleAccessToken).
					Return(httpclient.Response[access.GoogleTokenInfoResponse]{
						Code: http.StatusOK,
						Response: access.GoogleTokenInfoResponse{
							Email: "test@example.com",
						},
					}, nil)

				m.googleClient.
					EXPECT().
					GetUserProfile(args.ctx, args.req.GoogleAccessToken).
					Return(httpclient.Response[access.GoogleUserProfileResponse]{
						Code: http.StatusOK,
						Response: access.GoogleUserProfileResponse{
							Picture: "https://example.com/profile.jpg",
						},
					}, nil)

				m.googleClient.EXPECT().RevokeToken(args.ctx, args.req.GoogleAccessToken).
					Return(httpclient.Response[any]{
						Code: http.StatusOK,
					}, nil)

				m.hash.
					EXPECT().
					HashSha256EncodePepper("test@example.com").
					Return("hashed_email")

				m.memberStorage.
					EXPECT().
					GetMemberByEmail(args.ctx, "hashed_email").
					Return(access.Member{}, assert.AnError)
			},
			args: args{
				req: auth.ResolveIdentityRequest{
					GoogleAccessToken: "valid_google_access_token",
				},
			},
			want: want{
				err:     true,
				code:    app.CodeInternalError,
				Message: app.MessageInternalError,
				data: wrapper.ResponseOption[auth.ResolveIdentityResponse]{
					HTTPStatus: http.StatusInternalServerError,
					Code:       app.CodeInternalError,
					Message:    app.MessageInternalError,
				},
			},
		},
		{
			name: "fail, case organization storage GetMemberOrganization error",
			prepare: func(m mockArgs, args args) {
				m.googleClient.
					EXPECT().
					ValidateAccessToken(args.ctx, args.req.GoogleAccessToken).
					Return(httpclient.Response[access.GoogleTokenInfoResponse]{
						Code: http.StatusOK,
						Response: access.GoogleTokenInfoResponse{
							Email: "test@example.com",
						},
					}, nil)

				m.googleClient.
					EXPECT().
					GetUserProfile(args.ctx, args.req.GoogleAccessToken).
					Return(httpclient.Response[access.GoogleUserProfileResponse]{
						Code: http.StatusOK,
						Response: access.GoogleUserProfileResponse{
							Picture: "https://example.com/profile.jpg",
						},
					}, nil)

				m.googleClient.EXPECT().RevokeToken(args.ctx, args.req.GoogleAccessToken).
					Return(httpclient.Response[any]{
						Code: http.StatusOK,
					}, nil)

				m.hash.
					EXPECT().
					HashSha256EncodePepper("test@example.com").
					Return("hashed_email")

				m.memberStorage.
					EXPECT().
					GetMemberByEmail(args.ctx, "hashed_email").
					Return(access.Member{
						MemberID:       memberID,
						Username:       "testuser",
						EncryptedEmail: "encrypted_email",
						HashedEmail:    "hashed_email",
						Status:         access.MemberStatusActive,
						CreatedAt:      now,
						UpdatedAt:      now,
					}, nil)

				m.organizationStorage.
					EXPECT().
					GetMemberOrganization(args.ctx, uuid.MustParse(memberID)).
					Return(access.OrganizationMember{}, assert.AnError)
			},
			args: args{
				req: auth.ResolveIdentityRequest{
					GoogleAccessToken: "valid_google_access_token",
				},
			},
			want: want{
				err:     true,
				code:    app.CodeInternalError,
				Message: app.MessageInternalError,
				data: wrapper.ResponseOption[auth.ResolveIdentityResponse]{
					HTTPStatus: http.StatusInternalServerError,
					Code:       app.CodeInternalError,
					Message:    app.MessageInternalError,
				},
			},
		},
		{
			name: "fail, case aesgcm Decrypt error",
			prepare: func(m mockArgs, args args) {
				m.googleClient.
					EXPECT().
					ValidateAccessToken(args.ctx, args.req.GoogleAccessToken).
					Return(httpclient.Response[access.GoogleTokenInfoResponse]{
						Code: http.StatusOK,
						Response: access.GoogleTokenInfoResponse{
							Email: "test@example.com",
						},
					}, nil)

				m.googleClient.
					EXPECT().
					GetUserProfile(args.ctx, args.req.GoogleAccessToken).
					Return(httpclient.Response[access.GoogleUserProfileResponse]{
						Code: http.StatusOK,
						Response: access.GoogleUserProfileResponse{
							Picture: "https://example.com/profile.jpg",
						},
					}, nil)

				m.googleClient.EXPECT().RevokeToken(args.ctx, args.req.GoogleAccessToken).
					Return(httpclient.Response[any]{
						Code: http.StatusOK,
					}, nil)

				m.hash.
					EXPECT().
					HashSha256EncodePepper("test@example.com").
					Return("hashed_email")

				m.memberStorage.
					EXPECT().
					GetMemberByEmail(args.ctx, "hashed_email").
					Return(access.Member{
						MemberID:       memberID,
						Username:       "testuser",
						EncryptedEmail: "encrypted_email",
						HashedEmail:    "hashed_email",
						Status:         access.MemberStatusActive,
						CreatedAt:      now,
						UpdatedAt:      now,
					}, nil)

				m.organizationStorage.
					EXPECT().
					GetMemberOrganization(args.ctx, uuid.MustParse(memberID)).
					Return(access.OrganizationMember{
						OrganizationID: organizationID,
						MemberID:       memberID,
						Role:           access.OrganizationMemberRoleAdmin,
						Status:         access.OrganizationMemberStatusActive,
						CreatedAt:      now,
						UpdatedAt:      now,
					}, nil)

				m.aesgcm.
					EXPECT().
					Decrypt("encrypted_email").
					Return("", assert.AnError)
			},
			args: args{
				req: auth.ResolveIdentityRequest{
					GoogleAccessToken: "valid_google_access_token",
				},
			},
			want: want{
				err:     true,
				code:    app.CodeInternalError,
				Message: app.MessageInternalError,
				data: wrapper.ResponseOption[auth.ResolveIdentityResponse]{
					HTTPStatus: http.StatusInternalServerError,
					Code:       app.CodeInternalError,
					Message:    app.MessageInternalError,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(w)

			// Arrange
			var payload bytes.Buffer
			json.NewEncoder(&payload).Encode(tt.args.req)

			req := httptest.NewRequest(http.MethodPost, "http://0.0.0.0/api/v1/auth/resolve-identity", &payload)
			req.Header.Set("Content-Type", "application/json")

			ctx.Request = req

			m := mockArgs{
				googleClient:        access_mocks.NewGoogleClientMock(t),
				memberStorage:       access_mocks.NewMemberStorageMock(t),
				organizationStorage: access_mocks.NewOrganizationStorageMock(t),
				hash:                hash_mocks.NewHashManagerMock(t),
				aesgcm:              aesgcm_mocks.NewAesgcmMock(t),
			}

			if tt.prepare != nil {
				tt.args.ctx = ctx.Request.Context()
				tt.prepare(m, tt.args)
			}

			h := auth.NewHandler(auth.HandlerConfig{
				GoogleClient: m.googleClient,
				Hash:         m.hash,
				Aesgcm:       m.aesgcm,
			})

			// Act
			h.ResolveIdentify(ctx)

			// Assert
			var resp wrapper.ResponseOption[auth.ResolveIdentityResponse]
			json.NewDecoder(w.Body).Decode(&resp)

			if tt.want.err {
				r.NotEqual(http.StatusOK, w.Code)
				r.Equal(tt.want.code, resp.Code)
				r.Equal(tt.want.Message, resp.Message)
			} else {
				r.Equal(http.StatusOK, w.Code)
				r.Equal(tt.want.code, resp.Code)
				r.Equal(tt.want.Message, resp.Message)
				r.Equal(tt.want.data.Data, resp.Data)
			}
		})
	}
}
