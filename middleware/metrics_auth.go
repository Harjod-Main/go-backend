package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/RinTanth/go-common/app"
	"github.com/RinTanth/go-common/wrapper"
	"github.com/gin-gonic/gin"
)

// MetricsBearerAuth protects /metrics with a shared bearer token.
// When token is empty the route stays open (intended for LOCAL only).
func MetricsBearerAuth(token string) gin.HandlerFunc {
	expected := strings.TrimSpace(token)
	return func(c *gin.Context) {
		if expected == "" {
			c.Next()
			return
		}

		authHeader := c.GetHeader("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(authHeader, prefix) {
			wrapper.Respond(c, wrapper.ResponseOption[any]{
				HTTPStatus: http.StatusUnauthorized,
				Code:       app.CodeUnauthorized,
				Message:    app.MessageUnauthorized,
			})
			c.Abort()
			return
		}

		got := strings.TrimSpace(strings.TrimPrefix(authHeader, prefix))
		if subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
			wrapper.Respond(c, wrapper.ResponseOption[any]{
				HTTPStatus: http.StatusUnauthorized,
				Code:       app.CodeUnauthorized,
				Message:    app.MessageUnauthorized,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
