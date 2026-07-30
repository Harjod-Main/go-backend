package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RinTanth/go-backend/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDebugClientIP_DisabledByDefault(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	registerDebugRoutes(engine, config.Config{})

	req := httptest.NewRequest(http.MethodGet, "/debug/client-ip", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusNotFound, w.Code)
}

func TestDebugClientIP_DisabledInProdEvenWhenFlagSet(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	prev := config.Env
	t.Cleanup(func() { config.Env = prev })
	config.Env = config.Prod

	engine := gin.New()
	registerDebugRoutes(engine, config.Config{
		Server: config.Server{EnableDebugClientIP: true},
	})

	req := httptest.NewRequest(http.MethodGet, "/debug/client-ip", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusNotFound, w.Code)
}

func TestDebugClientIP_ReturnsRemoteAddrAndHeaders(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	prev := config.Env
	t.Cleanup(func() { config.Env = prev })
	config.Env = config.Local

	engine := gin.New()
	registerDebugRoutes(engine, config.Config{
		Server: config.Server{EnableDebugClientIP: true},
	})

	req := httptest.NewRequest(http.MethodGet, "/debug/client-ip", nil)
	req.RemoteAddr = "10.21.157.68:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 10.21.157.68")
	req.Header.Set("CF-Connecting-IP", "203.0.113.50")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusOK, w.Code)

	var body map[string]any
	r.NoError(json.Unmarshal(w.Body.Bytes(), &body))
	r.Equal("10.21.157.68:12345", body["remoteAddr"])
	r.Equal("203.0.113.50, 10.21.157.68", body["xForwardedFor"])
	r.Equal("203.0.113.50", body["cfConnectingIP"])
}
