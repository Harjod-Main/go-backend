package router

import (
	"net/http"
	"strings"

	"github.com/RinTanth/go-backend/config"
	"github.com/gin-gonic/gin"
)

// registerDebugRoutes adds temporary diagnostics for discovering TRUSTED_PROXY_CIDRS.
// Never registered in PROD, even if ENABLE_DEBUG_CLIENT_IP is set.
func registerDebugRoutes(r *gin.Engine, cfg config.Config) {
	if config.IsProdEnv() || !cfg.Server.EnableDebugClientIP {
		return
	}

	r.GET("/debug/client-ip", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"remoteAddr":     c.Request.RemoteAddr,
			"clientIP":       c.ClientIP(),
			"trustedProxies": strings.TrimSpace(cfg.Server.TrustedProxyCIDRs),
			"xForwardedFor":  c.GetHeader("X-Forwarded-For"),
			"xRealIP":        c.GetHeader("X-Real-IP"),
			"cfConnectingIP": c.GetHeader("CF-Connecting-IP"),
			"trueClientIP":   c.GetHeader("True-Client-IP"),
			"forwarded":      c.GetHeader("Forwarded"),
			"hint":           "Use remoteAddr host (before ':port') as the basis for TRUSTED_PROXY_CIDRS, then disable ENABLE_DEBUG_CLIENT_IP.",
		})
	})
}
