package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RinTanth/go-backend/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTrustedProxyCIDRs_Defaults(t *testing.T) {
	r := require.New(t)

	config.Env = "PROD"
	prod := trustedProxyCIDRs(config.Config{})
	r.Equal(defaultPrivateProxyCIDRs, prod)

	config.Env = "LOCAL"
	localCIDRs := trustedProxyCIDRs(config.Config{})
	r.Equal(defaultLocalProxyCIDRs, localCIDRs)
}

func TestTrustedProxyCIDRs_OverrideFromEnv(t *testing.T) {
	r := require.New(t)
	cidrs := trustedProxyCIDRs(config.Config{
		Server: config.Server{TrustedProxyCIDRs: " 10.0.0.0/8 , 192.168.0.0/16 "},
	})
	r.Equal([]string{"10.0.0.0/8", "192.168.0.0/16"}, cidrs)
}

func TestApplyTrustedProxies_UsesXForwardedForFromLB(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	err := applyTrustedProxies(engine, config.Config{
		Server: config.Server{TrustedProxyCIDRs: "10.0.0.0/8"},
	})
	r.NoError(err)

	engine.GET("/ip", func(c *gin.Context) {
		c.String(http.StatusOK, c.ClientIP())
	})

	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 10.0.0.1")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusOK, w.Code)
	r.Equal("203.0.113.50", w.Body.String())
}

func TestApplyTrustedProxies_IgnoresSpoofedXFFFromUntrustedPeer(t *testing.T) {
	r := require.New(t)
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	err := applyTrustedProxies(engine, config.Config{
		Server: config.Server{TrustedProxyCIDRs: "10.0.0.0/8"},
	})
	r.NoError(err)

	engine.GET("/ip", func(c *gin.Context) {
		c.String(http.StatusOK, c.ClientIP())
	})

	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "203.0.113.99:12345"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	r.Equal(http.StatusOK, w.Code)
	r.Equal("203.0.113.99", w.Body.String())
}
