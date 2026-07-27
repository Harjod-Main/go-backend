package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RinTanth/go-backend/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestIPRateLimit_AllowsThenBlocks(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.Use(middleware.IPRateLimit(2, time.Minute))
	engine.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	do := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.RemoteAddr = "203.0.113.10:12345"
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		return w
	}

	r.Equal(http.StatusOK, do().Code)
	r.Equal(http.StatusOK, do().Code)

	blocked := do()
	r.Equal(http.StatusTooManyRequests, blocked.Code)
	r.NotEmpty(blocked.Header().Get("Retry-After"))

	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	r.NoError(json.Unmarshal(blocked.Body.Bytes(), &body))
	r.Equal("HM400", body.Code)
	r.Equal("Too Many Requests", body.Message)
}

func TestIPRateLimit_PerClientBehindTrustedProxy(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	r.NoError(engine.SetTrustedProxies([]string{"10.0.0.0/8"}))
	engine.Use(middleware.IPRateLimit(2, time.Minute))
	engine.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	do := func(clientIP string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		req.Header.Set("X-Forwarded-For", clientIP)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		return w
	}

	r.Equal(http.StatusOK, do("203.0.113.1").Code)
	r.Equal(http.StatusOK, do("203.0.113.1").Code)
	r.Equal(http.StatusTooManyRequests, do("203.0.113.1").Code)

	r.Equal(http.StatusOK, do("203.0.113.2").Code)
}
