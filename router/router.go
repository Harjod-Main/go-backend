package router

import (
	"fmt"
	"time"

	"github.com/RinTanth/go-backend/app/auth"
	"github.com/RinTanth/go-backend/app/auth/supabaseauth"
	"github.com/RinTanth/go-backend/app/checkins"
	"github.com/RinTanth/go-backend/app/mediaurl"
	"github.com/RinTanth/go-backend/app/places"
	"github.com/RinTanth/go-backend/app/profile"
	"github.com/RinTanth/go-backend/app/reports"
	"github.com/RinTanth/go-backend/app/reviews"
	"github.com/RinTanth/go-backend/app/submissions"
	"github.com/RinTanth/go-backend/config"
	localmw "github.com/RinTanth/go-backend/middleware"
	"github.com/RinTanth/go-common/app"
	"github.com/RinTanth/go-common/health"
	"github.com/RinTanth/go-common/middleware"

	"github.com/gin-gonic/gin"
)

const (
	writeRateLimitPerMinute = 15
)

// New constructs a gin.Engine with routes and middleware configured.
func New(cfg config.Config, version, commit string, timeoutDuration time.Duration) (*gin.Engine, func(), error) {
	r := gin.New()
	if err := applyTrustedProxies(r, cfg); err != nil {
		return nil, nil, fmt.Errorf("apply trusted proxies: %w", err)
	}
	r.Use(gin.Recovery())

	if config.IsLocalEnv() {
		r.Use(gin.Logger())
	}

	r.GET("/liveness", health.Liveness(version, commit))
	r.GET("/readiness", health.Readiness())
	registerDebugRoutes(r, cfg)

	r.Use(
		middleware.SecurityHeaders(),
		localmw.CORS(cfg.AccessControl.AllowOrigin, allowedHeaders(cfg.Header.RefIDHeaderKey)),
		app.TraceContextTraceIDMiddleware(""),
		app.RefIDMiddleware(cfg.Header.RefIDHeaderKey),
		app.AutoLoggingMiddleware,
		middleware.Timeout(timeoutDuration),
		middleware.AccessLog(),
	)

	pool, err := newPostgresPool(cfg)
	if err != nil {
		return nil, nil, err
	}
	mediaurl.Configure(cfg.Supabase.ProjectURL)
	placesHandler := places.NewHandler(places.HandlerConfig{
		Repo:   places.NewPostgresRepo(pool),
		Google: places.NewGooglePlacesClient(cfg.GooglePlaces.APIKey),
	})

	profileRepo := profile.NewPostgresRepo(pool)
	profileHandler := profile.NewHandler(profile.HandlerConfig{Repo: profileRepo})
	reviewsHandler := reviews.NewHandler(reviews.HandlerConfig{
		Repo:     reviews.NewPostgresRepo(pool),
		Profiles: profileRepo,
	})
	reportsHandler := reports.NewHandler(reports.HandlerConfig{
		Repo: reports.NewPostgresRepo(pool),
	})

	verifier, err := supabaseauth.NewVerifier(
		cfg.Supabase.ProjectURL,
		cfg.Supabase.Audience,
	)
	if err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("create supabase verifier: %w", err)
	}

	authHandler := auth.NewHandler(auth.HandlerConfig{ProfileRepo: profileRepo})
	submissionsHandler := submissions.NewHandler(submissions.HandlerConfig{
		Repo:     submissions.NewPostgresRepo(pool),
		Profiles: profileRepo,
	})
	checkinsHandler := checkins.NewHandler(checkins.HandlerConfig{
		Repo:     checkins.NewPostgresRepo(pool),
		Profiles: profileRepo,
	})
	registerPlacesRoutes(r, placesHandler, verifier)
	registerAuthRoutes(r, authHandler, verifier)
	registerReviewsRoutes(r, reviewsHandler, verifier)
	registerReportsRoutes(r, reportsHandler, verifier)
	registerProfileRoutes(r, profileHandler, verifier)
	registerSubmissionsRoutes(r, submissionsHandler, verifier)
	registerCheckInsRoutes(r, checkinsHandler, verifier)

	return r, pool.Close, nil
}

func registerAuthRoutes(r *gin.Engine, authHandler *auth.Handler, verifier *supabaseauth.Verifier) {
	authGroup := r.Group("/api/v1/auth")
	{
		authGroup.GET("/me", supabaseauth.Middleware(verifier), authHandler.Me)
	}
}

func registerPlacesRoutes(r *gin.Engine, placesHandler *places.Handler, verifier *supabaseauth.Verifier) {
	placesGroup := r.Group("/api/v1/places")
	// Public map reads are unauthenticated — bound per-IP burst traffic.
	// Per-process limiter; see middleware.IPRateLimit scaling note before adding replicas.
	placesGroup.Use(localmw.IPRateLimit(60, time.Minute))
	{
		// Public read — map is available to guests (matches Supabase RLS public SELECT).
		placesGroup.GET("", placesHandler.List)
		placesGroup.GET("/autocomplete", placesHandler.Autocomplete)
		placesGroup.GET("/details/:placeId", placesHandler.GetPlaceDetails)
		placesGroup.GET("/:placeId/rate", placesHandler.GetRate)
		placesGroup.GET("/:placeId/privileges", placesHandler.GetPrivileges)
		placesGroup.GET("/:placeId/quote", placesHandler.GetQuote)
		placesGroup.GET("/:placeId/reaction", supabaseauth.OptionalMiddleware(verifier), placesHandler.GetReaction)
	}

	placeReactionWrites := r.Group("/api/v1/places/:placeId/reaction")
	placeReactionWrites.Use(supabaseauth.Middleware(verifier), localmw.ActorRateLimit(writeRateLimitPerMinute, time.Minute))
	{
		placeReactionWrites.PUT("", placesHandler.SetReaction)
		placeReactionWrites.DELETE("", placesHandler.ClearReaction)
	}

	// Public batch quote reads can fan out into heavy DB work, so keep them on the
	// same short per-IP limiter as other unauthenticated place lookups.
	r.POST("/api/v1/quotes", localmw.IPRateLimit(60, time.Minute), placesHandler.CreateQuotes)

	privilegesGroup := r.Group("/api/v1/privileges")
	privilegesGroup.Use(localmw.IPRateLimit(60, time.Minute))
	{
		privilegesGroup.GET("/:kind/:id", placesHandler.GetPrivilegeDetail)
	}
}

func registerReviewsRoutes(r *gin.Engine, reviewsHandler *reviews.Handler, verifier *supabaseauth.Verifier) {
	r.GET(
		"/api/v1/places/:placeId/reviews",
		localmw.IPRateLimit(60, time.Minute),
		supabaseauth.OptionalMiddleware(verifier),
		reviewsHandler.ListByPlace,
	)

	reviewWrites := r.Group("/api/v1/places/:placeId/reviews")
	reviewWrites.Use(supabaseauth.Middleware(verifier), localmw.ActorRateLimit(writeRateLimitPerMinute, time.Minute))
	{
		reviewWrites.POST("", reviewsHandler.Create)
	}

	reviewMutations := r.Group("/api/v1/reviews/:reviewId")
	reviewMutations.Use(supabaseauth.Middleware(verifier), localmw.ActorRateLimit(writeRateLimitPerMinute, time.Minute))
	{
		reviewMutations.PATCH("", reviewsHandler.Update)
		reviewMutations.PUT("/like", reviewsHandler.SetLike)
		reviewMutations.DELETE("/like", reviewsHandler.ClearLike)
	}
}

func registerReportsRoutes(r *gin.Engine, reportsHandler *reports.Handler, verifier *supabaseauth.Verifier) {
	r.GET("/api/v1/me/reports", supabaseauth.Middleware(verifier), reportsHandler.ListMine)

	reportsWrites := r.Group("/api/v1/reports")
	reportsWrites.Use(supabaseauth.OptionalMiddleware(verifier), localmw.ActorRateLimit(writeRateLimitPerMinute, time.Minute))
	{
		reportsWrites.POST("", reportsHandler.CreateIssueReport)
	}

	reviewReportWrites := r.Group("/api/v1/reviews/:reviewId/reports")
	reviewReportWrites.Use(supabaseauth.Middleware(verifier), localmw.ActorRateLimit(writeRateLimitPerMinute, time.Minute))
	{
		reviewReportWrites.POST("", reportsHandler.CreateReviewReport)
	}

	feedbackWrites := r.Group("/api/v1/places/:placeId/feedback")
	feedbackWrites.Use(supabaseauth.OptionalMiddleware(verifier), localmw.ActorRateLimit(writeRateLimitPerMinute, time.Minute))
	{
		feedbackWrites.POST("", reportsHandler.CreatePlaceFeedback)
	}
}

func registerProfileRoutes(r *gin.Engine, profileHandler *profile.Handler, verifier *supabaseauth.Verifier) {
	profileGroup := r.Group("/api/v1/profile")
	profileGroup.Use(supabaseauth.Middleware(verifier))
	{
		profileGroup.GET("", profileHandler.Get)
		profileGroup.PATCH("", profileHandler.Update)
	}
	// Public leaderboard; auth optional so signed-in users get self rank.
	r.GET(
		"/api/v1/leaderboard",
		localmw.IPRateLimit(60, time.Minute),
		supabaseauth.OptionalMiddleware(verifier),
		profileHandler.ListLeaderboard,
	)
}

func registerSubmissionsRoutes(r *gin.Engine, submissionsHandler *submissions.Handler, verifier *supabaseauth.Verifier) {
	// Static path registered separately so it does not collide with /places/:placeId/*.
	r.POST("/api/v1/places/submissions", supabaseauth.Middleware(verifier), submissionsHandler.Create)
}

func registerCheckInsRoutes(r *gin.Engine, checkinsHandler *checkins.Handler, verifier *supabaseauth.Verifier) {
	r.GET("/api/v1/me/check-ins", supabaseauth.Middleware(verifier), checkinsHandler.ListMine)
	r.POST("/api/v1/places/:placeId/check-ins", supabaseauth.Middleware(verifier), checkinsHandler.Create)
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
		"If-None-Match",
		"X-Requested-With",
		refIDHeaderKey,
	}
}
