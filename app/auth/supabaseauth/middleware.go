package supabaseauth

import (
	"context"
	"net/http"
	"strings"

	"github.com/RinTanth/go-common/app"
	"github.com/RinTanth/go-common/wrapper"
	"github.com/gin-gonic/gin"
)

type ctxKey struct{}

var claimsCtxKey = ctxKey{}

const (
	// CtxClaimsKey stores *Claims on gin.Context.
	CtxClaimsKey = "supabase_auth_claims"
	bearerPrefix = "Bearer "
)

// Middleware verifies Supabase Auth Bearer tokens and stores claims on the context.
func Middleware(verifier *Verifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !attachClaims(c, verifier, true) {
			return
		}
		c.Next()
	}
}

// OptionalMiddleware attaches claims when a Bearer token is present.
// Requests without Authorization continue as anonymous; invalid tokens are rejected.
func OptionalMiddleware(verifier *Verifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !attachClaims(c, verifier, false) {
			return
		}
		c.Next()
	}
}

func attachClaims(c *gin.Context, verifier *Verifier, required bool) bool {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" || !strings.HasPrefix(authHeader, bearerPrefix) {
		if required {
			wrapper.Respond(c, wrapper.ResponseOption[any]{
				HTTPStatus: http.StatusUnauthorized,
				Code:       app.CodeUnauthorized,
				Message:    app.MessageUnauthorized,
			})
			c.Abort()
			return false
		}
		return true
	}

	raw := strings.TrimSpace(strings.TrimPrefix(authHeader, bearerPrefix))
	claims, err := verifier.Verify(raw)
	if err != nil {
		wrapper.Respond(c, wrapper.ResponseOption[any]{
			HTTPStatus: http.StatusUnauthorized,
			Code:       app.CodeUnauthorized,
			Message:    app.MessageUnauthorized,
			Err:        err,
		})
		c.Abort()
		return false
	}

	c.Set(CtxClaimsKey, claims)
	ctx := context.WithValue(c.Request.Context(), claimsCtxKey, claims)
	c.Request = c.Request.WithContext(ctx)
	return true
}

// ClaimsFromContext returns Supabase claims from a request context.
func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	claims, ok := ctx.Value(claimsCtxKey).(*Claims)
	return claims, ok
}

// ClaimsFromGin returns Supabase claims from gin context.
func ClaimsFromGin(c *gin.Context) (*Claims, bool) {
	value, ok := c.Get(CtxClaimsKey)
	if !ok {
		return nil, false
	}
	claims, ok := value.(*Claims)
	return claims, ok
}
