package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RinTanth/go-backend/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMetricsBearerAuth_OpenWhenTokenEmpty(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.GET("/metrics", middleware.MetricsBearerAuth(""), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusOK, w.Code)
}

func TestMetricsBearerAuth_RejectsMissingOrWrongToken(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.GET("/metrics", middleware.MetricsBearerAuth("secret"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	do := func(auth string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		return w
	}

	r.Equal(http.StatusUnauthorized, do("").Code)
	r.Equal(http.StatusUnauthorized, do("Bearer wrong").Code)
	r.Equal(http.StatusOK, do("Bearer secret").Code)
}
