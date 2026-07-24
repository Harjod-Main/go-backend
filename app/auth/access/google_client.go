package access

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/RinTanth/go-common/httpclient"
)

type GoogleClienter interface {
	ValidateAccessToken(ctx context.Context, accessToken string) (httpclient.Response[GoogleTokenInfoResponse], error)
	GetUserProfile(ctx context.Context, accessToken string) (httpclient.Response[GoogleUserProfileResponse], error)
	RevokeToken(ctx context.Context, accessToken string) (httpclient.Response[any], error)
}

type googleClient struct {
	tokenInfoURL string
	userInfoURL  string
	revokeURL    string
	client       *http.Client
}

func NewGoogleClient(tokenInfoURL, userInfoURL, revokeURL string, client *http.Client) GoogleClienter {
	return &googleClient{
		tokenInfoURL: tokenInfoURL,
		userInfoURL:  userInfoURL,
		revokeURL:    revokeURL,
		client:       client,
	}
}

type GoogleTokenInfoResponse struct {
	Azp              string `json:"azp"`
	Aud              string `json:"aud"`
	Sub              string `json:"sub"`
	Scope            string `json:"scope"`
	Exp              string `json:"exp"`
	ExpiresIn        string `json:"expires_in"`
	Email            string `json:"email"`
	EmailVerified    string `json:"email_verified"`
	AccessType       string `json:"access_type"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

func (c *googleClient) ValidateAccessToken(ctx context.Context, accessToken string) (httpclient.Response[GoogleTokenInfoResponse], error) {
	// Prefer form body over query string so tokens are not logged in URLs
	// (Google migrated tokeninfo away from query params for the same reason).
	return postForm[GoogleTokenInfoResponse](ctx, c.client, c.tokenInfoURL, url.Values{
		"access_token": {accessToken},
	})
}

type GoogleUserProfileResponse struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"verified_email"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Picture       string `json:"picture"`
	Locale        string `json:"locale"`
}

func (c *googleClient) GetUserProfile(ctx context.Context, accessToken string) (httpclient.Response[GoogleUserProfileResponse], error) {
	return httpclient.GetWithOptions[GoogleUserProfileResponse](ctx, c.client, c.userInfoURL, httpclient.BearerTokenOption(accessToken))
}

func (c *googleClient) RevokeToken(ctx context.Context, accessToken string) (httpclient.Response[any], error) {
	return postForm[any](ctx, c.client, c.revokeURL, url.Values{
		"token": {accessToken},
	})
}

func postForm[RES any](ctx context.Context, client *http.Client, endpoint string, form url.Values) (httpclient.Response[RES], error) {
	body := strings.NewReader(form.Encode())
	return httpclient.PostWithOptions[*strings.Reader, RES](
		ctx,
		client,
		endpoint,
		body,
		httpclient.HeaderOption("Content-Type", "application/x-www-form-urlencoded"),
	)
}
