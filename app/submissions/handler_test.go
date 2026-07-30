package submissions_test

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
	"github.com/RinTanth/go-backend/app/submissions"
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
}

func (s *stubRepo) Create(_ context.Context, _ *submissions.Submission) error {
	s.createCalled = true
	return nil
}

func validSubmissionBody() map[string]any {
	return map[string]any{
		"name":          "Siam Paragon Parking",
		"latitude":      13.7466,
		"longitude":     100.5347,
		"amenities":     []string{"covered"},
		"photoUrls":     []string{testMediaPrefix + "11111111-1111-1111-1111-111111111111/submissions/a.jpg"},
		"ratePhotoUrls": []string{testMediaPrefix + "11111111-1111-1111-1111-111111111111/submissions/rate.jpg"},
	}
}

func performCreate(t *testing.T, payload []byte) (*httptest.ResponseRecorder, *stubRepo) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	repo := &stubRepo{}
	handler := submissions.NewHandler(submissions.HandlerConfig{Repo: repo})
	engine := gin.New()
	engine.POST("/api/v1/places/submissions", func(c *gin.Context) {
		c.Set(supabaseauth.CtxClaimsKey, &supabaseauth.Claims{Sub: "11111111-1111-1111-1111-111111111111"})
		handler.Create(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/places/submissions", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w, repo
}

func TestCreate_RejectsTooManyAmenities(t *testing.T) {
	r := require.New(t)

	body := validSubmissionBody()
	amenities := make([]string, 33)
	for i := range amenities {
		amenities[i] = "amenity"
	}
	body["amenities"] = amenities

	payload, err := json.Marshal(body)
	r.NoError(err)

	w, repo := performCreate(t, payload)
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(repo.createCalled)
}

func TestCreate_RejectsLongName(t *testing.T) {
	r := require.New(t)

	body := validSubmissionBody()
	body["name"] = strings.Repeat("a", 161)
	payload, err := json.Marshal(body)
	r.NoError(err)

	w, repo := performCreate(t, payload)
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(repo.createCalled)
}

func TestCreate_RejectsLargeJSONSection(t *testing.T) {
	r := require.New(t)

	body := validSubmissionBody()
	body["rateTiers"] = []map[string]string{
		{"notes": strings.Repeat("x", 33*1024)},
	}
	payload, err := json.Marshal(body)
	r.NoError(err)

	w, repo := performCreate(t, payload)
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(repo.createCalled)
}

func TestCreate_RejectsOversizedBody(t *testing.T) {
	r := require.New(t)

	body := validSubmissionBody()
	body["photoUrls"] = []string{testMediaPrefix + strings.Repeat("x", 260*1024)}
	payload, err := json.Marshal(body)
	r.NoError(err)

	w, repo := performCreate(t, payload)
	r.Equal(http.StatusBadRequest, w.Code)
	r.False(repo.createCalled)
}
