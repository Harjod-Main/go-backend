package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// DefaultCORSMethods are allowed for browser clients (profile uses PATCH).
const DefaultCORSMethods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"

// CORS sets Access-Control-* response headers.
//
// Replaces go-common middleware.AccessControl which incorrectly sets
// Access-Control-Request-Method (request header name) instead of
// Access-Control-Allow-Methods, and omits PATCH — blocking browser PATCH
// to /api/v1/profile after a successful OPTIONS preflight.
func CORS(allowOrigin string, allowedHeaders []string) gin.HandlerFunc {
	headers := strings.Join(allowedHeaders, ",")
	return func(c *gin.Context) {
		origin := strings.TrimSpace(allowOrigin)
		if origin != "" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		}
		c.Writer.Header().Set("Access-Control-Allow-Methods", DefaultCORSMethods)
		c.Writer.Header().Set("Access-Control-Allow-Headers", headers)
		// Drop the mistyped header if another middleware set it.
		c.Writer.Header().Del("Access-Control-Request-Method")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
