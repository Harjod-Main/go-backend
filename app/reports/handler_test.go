package reports_test

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
	"github.com/RinTanth/go-backend/app/pagination"
	"github.com/RinTanth/go-backend/app/reports"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const testMediaPrefix = "https://sycwdwymeirxowbrqdgd.supabase.co/storage/v1/object/public/media/"
const testReportPath = testUserID + "/reports/photo.jpg"
const testUserID = "11111111-1111-1111-1111-111111111111"
const testPlaceID = "22222222-2222-2222-2222-222222222222"
const testReviewID = "33333333-3333-3333-3333-333333333333"

func TestMain(m *testing.M) {
	mediaurl.Configure("https://sycwdwymeirxowbrqdgd.supabase.co")
	code := m.Run()
	mediaurl.ResetForTest()
	os.Exit(code)
}

type stubRepo struct {
	createIssueCalled bool
	createIssueErr    error
	createdIssue      *reports.IssueReport

	createReviewReportCalled bool
	createReviewReportErr    error
	createdReviewReport      *reports.ReviewReport

	createFeedbackCalled bool
	createFeedbackErr    error
	createdFeedback      *reports.PlaceFeedback

	reviewExists    bool
	reviewExistsErr error

	placeExists    bool
	placeExistsErr error
}

func (s *stubRepo) CreateIssueReport(_ context.Context, report *reports.IssueReport) error {
	s.createIssueCalled = true
	if s.createIssueErr != nil {
		return s.createIssueErr
	}
	report.ReportID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	s.createdIssue = report
	return nil
}

func (s *stubRepo) CreateReviewReport(_ context.Context, report *reports.ReviewReport) error {
	s.createReviewReportCalled = true
	if s.createReviewReportErr != nil {
		return s.createReviewReportErr
	}
	report.ReportID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	s.createdReviewReport = report
	return nil
}

func (s *stubRepo) CreatePlaceFeedback(_ context.Context, feedback *reports.PlaceFeedback) error {
	s.createFeedbackCalled = true
	if s.createFeedbackErr != nil {
		return s.createFeedbackErr
	}
	feedback.FeedbackID = "dddddddd-dddd-dddd-dddd-dddddddddddd"
	s.createdFeedback = feedback
	return nil
}

func (s *stubRepo) ListIssueReportsByUser(_ context.Context, _ string, _ int, _ *pagination.Cursor) ([]reports.IssueReport, *string, error) {
	return nil, nil, nil
}

func (s *stubRepo) ReviewExists(_ context.Context, _ string) (bool, error) {
	if s.reviewExistsErr != nil {
		return false, s.reviewExistsErr
	}
	return s.reviewExists, nil
}

func (s *stubRepo) PlaceExists(_ context.Context, _ string) (bool, error) {
	if s.placeExistsErr != nil {
		return false, s.placeExistsErr
	}
	return s.placeExists, nil
}

// --- CreateIssueReport ---

func validIssueReportBody() map[string]any {
	return map[string]any{
		"category":    "functional",
		"description": "The app crashes when I open the map.",
	}
}

func performCreateIssueReport(
	t *testing.T,
	repo *stubRepo,
	payload []byte,
	withAuth bool,
) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	handler := reports.NewHandler(reports.HandlerConfig{Repo: repo})
	engine := gin.New()
	engine.POST("/api/v1/reports", func(c *gin.Context) {
		if withAuth {
			c.Set(supabaseauth.CtxClaimsKey, &supabaseauth.Claims{
				Sub:   testUserID,
				Email: "reporter@example.com",
			})
		}
		handler.CreateIssueReport(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/reports", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func TestCreateIssueReport_SuccessUnauthenticated(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{}

	payload, err := json.Marshal(validIssueReportBody())
	r.NoError(err)

	w := performCreateIssueReport(t, repo, payload, false)
	r.Equal(http.StatusCreated, w.Code)
	r.True(repo.createIssueCalled)
	r.Nil(repo.createdIssue.UserID)
}

func TestCreateIssueReport_SuccessAuthenticatedAttachesUser(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{}

	payload, err := json.Marshal(validIssueReportBody())
	r.NoError(err)

	w := performCreateIssueReport(t, repo, payload, true)
	r.Equal(http.StatusCreated, w.Code)
	r.True(repo.createIssueCalled)
	r.NotNil(repo.createdIssue.UserID)
	r.Equal(testUserID, *repo.createdIssue.UserID)
	r.NotNil(repo.createdIssue.ReporterEmail)
	r.Equal("reporter@example.com", *repo.createdIssue.ReporterEmail)
}

func TestCreateIssueReport_RejectsInvalidCategory(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{}

	body := validIssueReportBody()
	body["category"] = "not-a-real-category"
	payload, err := json.Marshal(body)
	r.NoError(err)

	w := performCreateIssueReport(t, repo, payload, false)
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(repo.createIssueCalled)
}

func TestCreateIssueReport_RejectsEmptyDescription(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{}

	body := validIssueReportBody()
	body["description"] = "   "
	payload, err := json.Marshal(body)
	r.NoError(err)

	w := performCreateIssueReport(t, repo, payload, false)
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(repo.createIssueCalled)
}

func TestCreateIssueReport_RejectsDescriptionOverLengthCap(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{}

	body := validIssueReportBody()
	body["description"] = strings.Repeat("a", 4001)
	payload, err := json.Marshal(body)
	r.NoError(err)

	w := performCreateIssueReport(t, repo, payload, false)
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(repo.createIssueCalled)
}

func TestCreateIssueReport_AcceptsDescriptionAtLengthCap(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{}

	body := validIssueReportBody()
	body["description"] = strings.Repeat("a", 4000)
	payload, err := json.Marshal(body)
	r.NoError(err)

	w := performCreateIssueReport(t, repo, payload, false)
	r.Equal(http.StatusCreated, w.Code)
	r.True(repo.createIssueCalled)
}

func TestCreateIssueReport_RejectsOversizedBody(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{}

	body := validIssueReportBody()
	body["description"] = strings.Repeat("a", 150_000)
	payload, err := json.Marshal(body)
	r.NoError(err)

	w := performCreateIssueReport(t, repo, payload, false)
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(repo.createIssueCalled)
}

func TestCreateIssueReport_RejectsTooManyPhotoURLs(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{}

	body := validIssueReportBody()
	urls := make([]string, 6)
	for i := range urls {
		urls[i] = testReportPath
	}
	body["photoUrls"] = urls
	payload, err := json.Marshal(body)
	r.NoError(err)

	w := performCreateIssueReport(t, repo, payload, true)
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(repo.createIssueCalled)
}

func TestCreateIssueReport_AcceptsPrivateReportPaths(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{}

	body := validIssueReportBody()
	body["photoUrls"] = []string{testReportPath}
	payload, err := json.Marshal(body)
	r.NoError(err)

	w := performCreateIssueReport(t, repo, payload, true)
	r.Equal(http.StatusCreated, w.Code)
	r.True(repo.createIssueCalled)
	r.Equal([]string{testReportPath}, repo.createdIssue.PhotoURLs)
}

func TestCreateIssueReport_RejectsPhotosWithoutAuth(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{}

	body := validIssueReportBody()
	body["photoUrls"] = []string{testReportPath}
	payload, err := json.Marshal(body)
	r.NoError(err)

	w := performCreateIssueReport(t, repo, payload, false)
	r.Equal(http.StatusUnauthorized, w.Code)
	r.False(repo.createIssueCalled)
}

func TestCreateIssueReport_RejectsOtherUserPrivatePath(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{}

	body := validIssueReportBody()
	body["photoUrls"] = []string{"22222222-2222-2222-2222-222222222222/reports/private.jpg"}
	payload, err := json.Marshal(body)
	r.NoError(err)

	w := performCreateIssueReport(t, repo, payload, true)
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(repo.createIssueCalled)
}

func TestCreateIssueReport_RejectsPublicPhotoURLs(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{}

	body := validIssueReportBody()
	body["photoUrls"] = []string{testMediaPrefix + testUserID + "/reports/photo.jpg"}
	payload, err := json.Marshal(body)
	r.NoError(err)

	w := performCreateIssueReport(t, repo, payload, true)
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(repo.createIssueCalled)
}

func TestCreateIssueReport_RepositoryErrorReturns500(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{createIssueErr: context.DeadlineExceeded}

	payload, err := json.Marshal(validIssueReportBody())
	r.NoError(err)

	w := performCreateIssueReport(t, repo, payload, false)
	r.Equal(http.StatusInternalServerError, w.Code)
	r.True(repo.createIssueCalled)
}

// --- CreateReviewReport ---

func validReviewReportBody() map[string]any {
	return map[string]any{
		"reason": "spam",
	}
}

func performCreateReviewReport(
	t *testing.T,
	repo *stubRepo,
	reviewID string,
	payload []byte,
	withAuth bool,
) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	handler := reports.NewHandler(reports.HandlerConfig{Repo: repo})
	engine := gin.New()
	engine.POST("/api/v1/reviews/:reviewId/reports", func(c *gin.Context) {
		if withAuth {
			c.Set(supabaseauth.CtxClaimsKey, &supabaseauth.Claims{Sub: testUserID})
		}
		handler.CreateReviewReport(c)
	})

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/reviews/"+reviewID+"/reports",
		bytes.NewReader(payload),
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func TestCreateReviewReport_Success(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{reviewExists: true}

	payload, err := json.Marshal(validReviewReportBody())
	r.NoError(err)

	w := performCreateReviewReport(t, repo, testReviewID, payload, true)
	r.Equal(http.StatusCreated, w.Code)
	r.True(repo.createReviewReportCalled)
	r.Equal(testUserID, *repo.createdReviewReport.UserID)
}

func TestCreateReviewReport_RejectsUnauthenticated(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{reviewExists: true}

	payload, err := json.Marshal(validReviewReportBody())
	r.NoError(err)

	w := performCreateReviewReport(t, repo, testReviewID, payload, false)
	r.Equal(http.StatusUnauthorized, w.Code)
	r.False(repo.createReviewReportCalled)
}

func TestCreateReviewReport_RejectsInvalidReviewID(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{reviewExists: true}

	payload, err := json.Marshal(validReviewReportBody())
	r.NoError(err)

	w := performCreateReviewReport(t, repo, "not-a-uuid", payload, true)
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(repo.createReviewReportCalled)
}

func TestCreateReviewReport_RejectsInvalidReason(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{reviewExists: true}

	body := validReviewReportBody()
	body["reason"] = "not-a-real-reason"
	payload, err := json.Marshal(body)
	r.NoError(err)

	w := performCreateReviewReport(t, repo, testReviewID, payload, true)
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(repo.createReviewReportCalled)
}

func TestCreateReviewReport_RejectsMissingReview(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{reviewExists: false}

	payload, err := json.Marshal(validReviewReportBody())
	r.NoError(err)

	w := performCreateReviewReport(t, repo, testReviewID, payload, true)
	r.Equal(http.StatusNotFound, w.Code)
	r.False(repo.createReviewReportCalled)
}

func TestCreateReviewReport_ReviewExistsErrorReturns500(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{reviewExistsErr: context.DeadlineExceeded}

	payload, err := json.Marshal(validReviewReportBody())
	r.NoError(err)

	w := performCreateReviewReport(t, repo, testReviewID, payload, true)
	r.Equal(http.StatusInternalServerError, w.Code)
	r.False(repo.createReviewReportCalled)
}

func TestCreateReviewReport_RepositoryErrorReturns500(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{reviewExists: true, createReviewReportErr: context.DeadlineExceeded}

	payload, err := json.Marshal(validReviewReportBody())
	r.NoError(err)

	w := performCreateReviewReport(t, repo, testReviewID, payload, true)
	r.Equal(http.StatusInternalServerError, w.Code)
	r.True(repo.createReviewReportCalled)
}

func TestCreateReviewReport_RejectsDetailOverLengthCap(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{reviewExists: true}

	body := validReviewReportBody()
	body["detail"] = strings.Repeat("a", 1001)
	payload, err := json.Marshal(body)
	r.NoError(err)

	w := performCreateReviewReport(t, repo, testReviewID, payload, true)
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(repo.createReviewReportCalled)
}

// --- CreatePlaceFeedback ---

func validPlaceFeedbackBody() map[string]any {
	return map[string]any{
		"feedbackType": "wrong_price",
	}
}

func performCreatePlaceFeedback(
	t *testing.T,
	repo *stubRepo,
	placeID string,
	payload []byte,
	withAuth bool,
) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	handler := reports.NewHandler(reports.HandlerConfig{Repo: repo})
	engine := gin.New()
	engine.POST("/api/v1/places/:placeId/feedback", func(c *gin.Context) {
		if withAuth {
			c.Set(supabaseauth.CtxClaimsKey, &supabaseauth.Claims{
				Sub:   testUserID,
				Email: "feedback@example.com",
			})
		}
		handler.CreatePlaceFeedback(c)
	})

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/places/"+placeID+"/feedback",
		bytes.NewReader(payload),
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func TestCreatePlaceFeedback_SuccessUnauthenticated(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{placeExists: true}

	payload, err := json.Marshal(validPlaceFeedbackBody())
	r.NoError(err)

	w := performCreatePlaceFeedback(t, repo, testPlaceID, payload, false)
	r.Equal(http.StatusCreated, w.Code)
	r.True(repo.createFeedbackCalled)
	r.Nil(repo.createdFeedback.UserID)
}

func TestCreatePlaceFeedback_SuccessAuthenticatedAttachesUser(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{placeExists: true}

	payload, err := json.Marshal(validPlaceFeedbackBody())
	r.NoError(err)

	w := performCreatePlaceFeedback(t, repo, testPlaceID, payload, true)
	r.Equal(http.StatusCreated, w.Code)
	r.NotNil(repo.createdFeedback.UserID)
	r.Equal(testUserID, *repo.createdFeedback.UserID)
}

func TestCreatePlaceFeedback_RejectsInvalidPlaceID(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{placeExists: true}

	payload, err := json.Marshal(validPlaceFeedbackBody())
	r.NoError(err)

	w := performCreatePlaceFeedback(t, repo, "not-a-uuid", payload, false)
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(repo.createFeedbackCalled)
}

func TestCreatePlaceFeedback_RejectsInvalidFeedbackType(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{placeExists: true}

	body := validPlaceFeedbackBody()
	body["feedbackType"] = "not-a-real-type"
	payload, err := json.Marshal(body)
	r.NoError(err)

	w := performCreatePlaceFeedback(t, repo, testPlaceID, payload, false)
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(repo.createFeedbackCalled)
}

func TestCreatePlaceFeedback_RejectsMissingPlace(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{placeExists: false}

	payload, err := json.Marshal(validPlaceFeedbackBody())
	r.NoError(err)

	w := performCreatePlaceFeedback(t, repo, testPlaceID, payload, false)
	r.Equal(http.StatusNotFound, w.Code)
	r.False(repo.createFeedbackCalled)
}

func TestCreatePlaceFeedback_PlaceExistsErrorReturns500(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{placeExistsErr: context.DeadlineExceeded}

	payload, err := json.Marshal(validPlaceFeedbackBody())
	r.NoError(err)

	w := performCreatePlaceFeedback(t, repo, testPlaceID, payload, false)
	r.Equal(http.StatusInternalServerError, w.Code)
	r.False(repo.createFeedbackCalled)
}

func TestCreatePlaceFeedback_RejectsTooManyPhotoURLs(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{placeExists: true}

	body := validPlaceFeedbackBody()
	urls := make([]string, 6)
	for i := range urls {
		urls[i] = testMediaPrefix + testUserID + "/feedback/photo.jpg"
	}
	body["photoUrls"] = urls
	payload, err := json.Marshal(body)
	r.NoError(err)

	w := performCreatePlaceFeedback(t, repo, testPlaceID, payload, false)
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(repo.createFeedbackCalled)
}

func TestCreatePlaceFeedback_RejectsInvalidSinglePhotoURL(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{placeExists: true}

	body := validPlaceFeedbackBody()
	body["photoUrl"] = "https://evil.example.com/not-allowed.jpg"
	payload, err := json.Marshal(body)
	r.NoError(err)

	w := performCreatePlaceFeedback(t, repo, testPlaceID, payload, false)
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(repo.createFeedbackCalled)
}

func TestCreatePlaceFeedback_BlankSinglePhotoURLTreatedAsAbsent(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{placeExists: true}

	body := validPlaceFeedbackBody()
	body["photoUrl"] = "   "
	payload, err := json.Marshal(body)
	r.NoError(err)

	w := performCreatePlaceFeedback(t, repo, testPlaceID, payload, false)
	r.Equal(http.StatusCreated, w.Code)
	r.True(repo.createFeedbackCalled)
	r.Nil(repo.createdFeedback.PhotoURL)
}

func TestCreatePlaceFeedback_RepositoryErrorReturns500(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{placeExists: true, createFeedbackErr: context.DeadlineExceeded}

	payload, err := json.Marshal(validPlaceFeedbackBody())
	r.NoError(err)

	w := performCreatePlaceFeedback(t, repo, testPlaceID, payload, false)
	r.Equal(http.StatusInternalServerError, w.Code)
	r.True(repo.createFeedbackCalled)
}

func TestCreatePlaceFeedback_RejectsDescriptionOverLengthCap(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{placeExists: true}

	body := validPlaceFeedbackBody()
	body["description"] = strings.Repeat("a", 50_000)
	payload, err := json.Marshal(body)
	r.NoError(err)

	w := performCreatePlaceFeedback(t, repo, testPlaceID, payload, false)
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(repo.createFeedbackCalled)
}

func TestCreatePlaceFeedback_RejectsOversizedBody(t *testing.T) {
	r := require.New(t)
	repo := &stubRepo{placeExists: true}

	body := validPlaceFeedbackBody()
	body["description"] = strings.Repeat("a", 150_000)
	payload, err := json.Marshal(body)
	r.NoError(err)

	w := performCreatePlaceFeedback(t, repo, testPlaceID, payload, false)
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(repo.createFeedbackCalled)
}
