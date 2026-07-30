package reviews_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/RinTanth/go-backend/app/auth/supabaseauth"
	"github.com/RinTanth/go-backend/app/mediaurl"
	"github.com/RinTanth/go-backend/app/profile"
	"github.com/RinTanth/go-backend/app/reviews"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const testMediaPrefix = "https://sycwdwymeirxowbrqdgd.supabase.co/storage/v1/object/public/media/"

func TestMain(m *testing.M) {
	mediaurl.Configure("https://sycwdwymeirxowbrqdgd.supabase.co")
	code := m.Run()
	mediaurl.ResetForTest()
	os.Exit(code)
}

type stubRepo struct {
	createCalled bool
	created      *reviews.Review
	createErr    error

	listItems []reviews.Review
	listTotal int
	listErr   error
}

func (s *stubRepo) ListByPlace(_ context.Context, _ string, _ int, _ int) ([]reviews.Review, int, error) {
	if s.listErr != nil {
		return nil, 0, s.listErr
	}
	return s.listItems, s.listTotal, nil
}

func (s *stubRepo) Create(_ context.Context, review *reviews.Review) error {
	s.createCalled = true
	if s.createErr != nil {
		return s.createErr
	}
	review.ReviewID = "cccccccc-cccc-cccc-cccc-cccccccccccc"
	s.created = review
	return nil
}

const testPlaceID = "22222222-2222-2222-2222-222222222222"
const testUserID = "11111111-1111-1111-1111-111111111111"

// stubProfiles is a minimal profile.Repository stub. Ensure returns profileOut
// (or profileErr, if set); the other methods are unused by reviews.Handler and
// only exist to satisfy the interface.
type stubProfiles struct {
	ensureCalled bool
	profileOut   *profile.Profile
	profileErr   error
}

func (s *stubProfiles) GetByUserID(context.Context, string) (*profile.Profile, error) {
	return nil, profile.ErrNotFound
}

func (s *stubProfiles) Ensure(_ context.Context, userID, _ string, _ profile.OAuthSeed) (*profile.Profile, error) {
	s.ensureCalled = true
	if s.profileErr != nil {
		return nil, s.profileErr
	}
	if s.profileOut != nil {
		return s.profileOut, nil
	}
	return &profile.Profile{UserID: userID, DisplayName: "u", Username: "user"}, nil
}

func (s *stubProfiles) SyncFromOAuth(context.Context, string, string, profile.OAuthSeed) (*profile.Profile, error) {
	return nil, profile.ErrNotFound
}

func (s *stubProfiles) Update(context.Context, string, *string, *string, *string, bool) (*profile.Profile, error) {
	return nil, profile.ErrNotFound
}

func validCreateBody() map[string]any {
	return map[string]any{
		"placeId": testPlaceID,
		"rating":  4,
	}
}

func performCreate(
	t *testing.T,
	repo *stubRepo,
	placeID string,
	payload []byte,
	withAuth bool,
) *httptest.ResponseRecorder {
	t.Helper()
	return performCreateWithProfiles(t, repo, nil, placeID, payload, withAuth)
}

func performCreateWithProfiles(
	t *testing.T,
	repo *stubRepo,
	profiles profile.Repository,
	placeID string,
	payload []byte,
	withAuth bool,
) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	handler := reviews.NewHandler(reviews.HandlerConfig{Repo: repo, Profiles: profiles})
	engine := gin.New()
	engine.POST("/api/v1/places/:placeId/reviews", func(c *gin.Context) {
		if withAuth {
			c.Set(supabaseauth.CtxClaimsKey, &supabaseauth.Claims{
				Sub:   testUserID,
				Email: "jane.doe@example.com",
			})
		}
		handler.Create(c)
	})

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/places/"+placeID+"/reviews",
		bytes.NewReader(payload),
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func TestCreate_Success(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{}

	payload, err := json.Marshal(validCreateBody())
	r.NoError(err)

	w := performCreate(t, repo, testPlaceID, payload, true)
	r.Equal(http.StatusCreated, w.Code)
	r.True(repo.createCalled)
	r.NotNil(repo.created)
	r.Equal(testUserID, repo.created.UserID)
	r.Equal("jane.doe", repo.created.DisplayName)
}

func TestCreate_RejectsUnauthenticated(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{}

	payload, err := json.Marshal(validCreateBody())
	r.NoError(err)

	w := performCreate(t, repo, testPlaceID, payload, false)
	r.Equal(http.StatusUnauthorized, w.Code)
	r.False(repo.createCalled)
}

func TestCreate_RejectsInvalidPlaceID(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{}

	payload, err := json.Marshal(validCreateBody())
	r.NoError(err)

	w := performCreate(t, repo, "not-a-uuid", payload, true)
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(repo.createCalled)
}

func TestCreate_RejectsMissingRating(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{}

	body := validCreateBody()
	delete(body, "rating")
	payload, err := json.Marshal(body)
	r.NoError(err)

	w := performCreate(t, repo, testPlaceID, payload, true)
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(repo.createCalled)
}

func TestCreate_RejectsRatingOutOfRange(t *testing.T) {
	r := require.New(t)

	for _, rating := range []int{0, 6, -1} {
		repo := &stubRepo{}
		body := validCreateBody()
		body["rating"] = rating
		payload, err := json.Marshal(body)
		r.NoError(err)

		w := performCreate(t, repo, testPlaceID, payload, true)
		r.Equal(http.StatusBadRequest, w.Code, "rating=%d should be rejected", rating)
		r.False(repo.createCalled)
	}
}

func TestCreate_RejectsTooManyPhotoURLs(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{}

	body := validCreateBody()
	urls := make([]string, 6)
	for i := range urls {
		urls[i] = testMediaPrefix + testUserID + "/reviews/photo.jpg"
	}
	body["photoUrls"] = urls
	payload, err := json.Marshal(body)
	r.NoError(err)

	w := performCreate(t, repo, testPlaceID, payload, true)
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(repo.createCalled)
}

func TestCreate_RejectsInvalidPhotoURL(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{}

	body := validCreateBody()
	body["photoUrls"] = []string{"https://evil.example.com/not-allowed.jpg"}
	payload, err := json.Marshal(body)
	r.NoError(err)

	w := performCreate(t, repo, testPlaceID, payload, true)
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(repo.createCalled)
}

func TestCreate_DisplayNameFallsBackToEmailLocalPartWithoutProfiles(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{}

	payload, err := json.Marshal(validCreateBody())
	r.NoError(err)

	// No Profiles repository wired — resolveDisplayName must fall back to the
	// email-derived name rather than panicking on a nil h.profiles.
	w := performCreate(t, repo, testPlaceID, payload, true)
	r.Equal(http.StatusCreated, w.Code)
	r.Equal("jane.doe", repo.created.DisplayName)
}

func TestCreate_DisplayNameUsesProfileDisplayName(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{}
	profiles := &stubProfiles{profileOut: &profile.Profile{
		UserID:      testUserID,
		DisplayName: "Jane Public Name",
		Username:    "janedoe",
	}}

	payload, err := json.Marshal(validCreateBody())
	r.NoError(err)

	w := performCreateWithProfiles(t, repo, profiles, testPlaceID, payload, true)
	r.Equal(http.StatusCreated, w.Code)
	r.True(profiles.ensureCalled)
	r.Equal("Jane Public Name", repo.created.DisplayName)
}

func TestCreate_DisplayNameFallsBackWhenProfileLookupFails(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{}
	profiles := &stubProfiles{profileErr: context.DeadlineExceeded}

	payload, err := json.Marshal(validCreateBody())
	r.NoError(err)

	w := performCreateWithProfiles(t, repo, profiles, testPlaceID, payload, true)
	r.Equal(http.StatusCreated, w.Code)
	r.True(profiles.ensureCalled)
	r.True(repo.createCalled)
	r.Equal("jane.doe", repo.created.DisplayName)
}

func TestCreate_DisplayNameFallsBackWhenProfileDisplayNameBlank(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{}
	profiles := &stubProfiles{profileOut: &profile.Profile{
		UserID:      testUserID,
		DisplayName: "   ",
		Username:    "janedoe",
	}}

	payload, err := json.Marshal(validCreateBody())
	r.NoError(err)

	w := performCreateWithProfiles(t, repo, profiles, testPlaceID, payload, true)
	r.Equal(http.StatusCreated, w.Code)
	r.Equal("jane.doe", repo.created.DisplayName)
}

func TestCreate_RepositoryErrorReturns500(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{createErr: context.DeadlineExceeded}

	payload, err := json.Marshal(validCreateBody())
	r.NoError(err)

	w := performCreate(t, repo, testPlaceID, payload, true)
	r.Equal(http.StatusInternalServerError, w.Code)
	r.True(repo.createCalled)
}

func performList(t *testing.T, repo *stubRepo, placeID string, query string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	handler := reviews.NewHandler(reviews.HandlerConfig{Repo: repo})
	engine := gin.New()
	engine.GET("/api/v1/places/:placeId/reviews", handler.ListByPlace)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/places/"+placeID+"/reviews"+query, nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func TestListByPlace_Success(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{
		listItems: []reviews.Review{{
			ReviewID:    "cccccccc-cccc-cccc-cccc-cccccccccccc",
			PlaceID:     testPlaceID,
			UserID:      testUserID,
			DisplayName: "jane.doe",
			Rating:      5,
		}},
		listTotal: 1,
	}

	w := performList(t, repo, testPlaceID, "")
	r.Equal(http.StatusOK, w.Code)

	var body struct {
		Data reviews.ReviewListResponse `json:"data"`
	}
	r.NoError(json.Unmarshal(w.Body.Bytes(), &body))
	r.Equal(1, body.Data.TotalCount)
	r.Len(body.Data.Reviews, 1)
}

func TestListByPlace_RejectsInvalidPlaceID(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{}

	w := performList(t, repo, "not-a-uuid", "")
	r.Equal(http.StatusBadRequest, w.Code)
}

func TestListByPlace_IgnoresOutOfRangeLimit(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{listItems: []reviews.Review{}, listTotal: 0}

	w := performList(t, repo, testPlaceID, "?limit=0&offset=-5")
	r.Equal(http.StatusOK, w.Code)
}

func TestListByPlace_RepositoryErrorReturns500(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{listErr: context.DeadlineExceeded}

	w := performList(t, repo, testPlaceID, "")
	r.Equal(http.StatusInternalServerError, w.Code)
}

func TestCreate_RejectsDescriptionOverLengthCap(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{}

	body := validCreateBody()
	body["description"] = strings.Repeat("a", 50_000)
	payload, err := json.Marshal(body)
	r.NoError(err)

	w := performCreate(t, repo, testPlaceID, payload, true)
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(repo.createCalled)
}

func TestCreate_RejectsOversizedBody(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{}

	body := validCreateBody()
	body["description"] = strings.Repeat("a", 150_000)
	payload, err := json.Marshal(body)
	r.NoError(err)

	w := performCreate(t, repo, testPlaceID, payload, true)
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(repo.createCalled)
}
