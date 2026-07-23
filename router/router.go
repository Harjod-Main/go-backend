package router

import (
	"time"

	"github.com/RinTanth/go-backend/app/auth"
	"github.com/RinTanth/go-backend/app/auth/supabaseauth"
	"github.com/RinTanth/go-backend/config"
	"github.com/RinTanth/go-common/app"
	"github.com/RinTanth/go-common/health"
	"github.com/RinTanth/go-common/middleware"

	"github.com/gin-gonic/gin"
)

// New constructs a gin.Engine with routes and middleware configured.
func New(cfg config.Config, version, commit string, timeoutDuration time.Duration) (*gin.Engine, func()) {
	r := gin.New()
	r.Use(gin.Recovery())

	if config.IsLocalEnv() {
		r.Use(gin.Logger())
	}

	r.GET("/liveness", health.Liveness(version, commit))
	r.GET("/metrics", health.Metrics())
	r.GET("/readiness", health.Readiness())

	r.Use(
		middleware.SecurityHeaders(),
		middleware.AccessControl(cfg.AccessControl.AllowOrigin, allowedHeaders(cfg.Header.RefIDHeaderKey)),
		app.TraceContextTraceIDMiddleware(""),
		app.RefIDMiddleware(cfg.Header.RefIDHeaderKey),
		app.AutoLoggingMiddleware,
		middleware.Timeout(timeoutDuration),
		middleware.AccessLog(),
	)

	// Postgres pool is deferred until places/quotes routes are registered.
	// Auth JWT verification does not touch the database.

	verifier, err := supabaseauth.NewVerifier(
		cfg.Supabase.JWTSecret,
		cfg.Supabase.ProjectURL,
		cfg.Supabase.Audience,
	)
	if err != nil {
		panic(err)
	}

	authHandler := auth.NewHandler(auth.HandlerConfig{})
	registerAuthRoutes(r, authHandler, verifier)

	return r, func() {}
}

func registerAuthRoutes(r *gin.Engine, authHandler *auth.Handler, verifier *supabaseauth.Verifier) {
	authGroup := r.Group("/api/v1/auth")
	{
		authGroup.GET("/me", supabaseauth.Middleware(verifier), authHandler.Me)
	}
}

func allowedHeaders(refIDHeaderKey string) []string {
	return []string{
		"Content-Type",
		"Content-Length",
		"Accept-Encoding",
		"X-CSRF-Token",
		"Authorization",
		"accept",
		"origin",
		"Cache-Control",
		"X-Requested-With",
		refIDHeaderKey,
	}
}
