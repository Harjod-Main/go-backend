package router

import (
	"time"

	"github.com/RinTanth/go-backend/app/auth"
	"github.com/RinTanth/go-backend/app/auth/supabaseauth"
	"github.com/RinTanth/go-backend/app/places"
	"github.com/RinTanth/go-backend/app/profile"
	"github.com/RinTanth/go-backend/app/reports"
	"github.com/RinTanth/go-backend/app/reviews"
	"github.com/RinTanth/go-backend/app/submissions"
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

	pool := newPostgresPool(cfg)
	placesHandler := places.NewHandler(places.HandlerConfig{
		Repo: places.NewPostgresRepo(pool),
	})
	registerPlacesRoutes(r, placesHandler)

	reviewsHandler := reviews.NewHandler(reviews.HandlerConfig{
		Repo: reviews.NewPostgresRepo(pool),
	})
	reportsHandler := reports.NewHandler(reports.HandlerConfig{
		Repo: reports.NewPostgresRepo(pool),
	})
	profileRepo := profile.NewPostgresRepo(pool)
	profileHandler := profile.NewHandler(profile.HandlerConfig{Repo: profileRepo})

	verifier, err := supabaseauth.NewVerifier(
		cfg.Supabase.ProjectURL,
		cfg.Supabase.Audience,
	)
	if err != nil {
		pool.Close()
		panic(err)
	}

	authHandler := auth.NewHandler(auth.HandlerConfig{ProfileRepo: profileRepo})
	submissionsHandler := submissions.NewHandler(submissions.HandlerConfig{
		Repo: submissions.NewPostgresRepo(pool),
	})
	registerAuthRoutes(r, authHandler, verifier)
	registerReviewsRoutes(r, reviewsHandler, verifier)
	registerReportsRoutes(r, reportsHandler, verifier)
	registerProfileRoutes(r, profileHandler, verifier)
	registerSubmissionsRoutes(r, submissionsHandler, verifier)

	return r, pool.Close
}

func registerAuthRoutes(r *gin.Engine, authHandler *auth.Handler, verifier *supabaseauth.Verifier) {
	authGroup := r.Group("/api/v1/auth")
	{
		authGroup.GET("/me", supabaseauth.Middleware(verifier), authHandler.Me)
	}
}

func registerPlacesRoutes(r *gin.Engine, placesHandler *places.Handler) {
	placesGroup := r.Group("/api/v1/places")
	{
		// Public read — map is available to guests (matches Supabase RLS public SELECT).
		placesGroup.GET("", placesHandler.List)
		placesGroup.GET("/:placeId/rate", placesHandler.GetRate)
		placesGroup.GET("/:placeId/privileges", placesHandler.GetPrivileges)
		placesGroup.GET("/:placeId/quote", placesHandler.GetQuote)
	}

	r.POST("/api/v1/quotes", placesHandler.CreateQuotes)

	privilegesGroup := r.Group("/api/v1/privileges")
	{
		privilegesGroup.GET("/:kind/:id", placesHandler.GetPrivilegeDetail)
	}
}

func registerReviewsRoutes(r *gin.Engine, reviewsHandler *reviews.Handler, verifier *supabaseauth.Verifier) {
	r.GET("/api/v1/places/:placeId/reviews", reviewsHandler.ListByPlace)
	r.POST("/api/v1/places/:placeId/reviews", supabaseauth.Middleware(verifier), reviewsHandler.Create)
}

func registerReportsRoutes(r *gin.Engine, reportsHandler *reports.Handler, verifier *supabaseauth.Verifier) {
	r.POST("/api/v1/reports", supabaseauth.OptionalMiddleware(verifier), reportsHandler.CreateIssueReport)
	r.POST("/api/v1/reviews/:reviewId/reports", supabaseauth.Middleware(verifier), reportsHandler.CreateReviewReport)
	r.POST("/api/v1/places/:placeId/feedback", supabaseauth.OptionalMiddleware(verifier), reportsHandler.CreatePlaceFeedback)
}

func registerProfileRoutes(r *gin.Engine, profileHandler *profile.Handler, verifier *supabaseauth.Verifier) {
	profileGroup := r.Group("/api/v1/profile")
	profileGroup.Use(supabaseauth.Middleware(verifier))
	{
		profileGroup.GET("", profileHandler.Get)
		profileGroup.PATCH("", profileHandler.Update)
	}
}

func registerSubmissionsRoutes(r *gin.Engine, submissionsHandler *submissions.Handler, verifier *supabaseauth.Verifier) {
	// Static path registered separately so it does not collide with /places/:placeId/*.
	r.POST("/api/v1/places/submissions", supabaseauth.Middleware(verifier), submissionsHandler.Create)
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
