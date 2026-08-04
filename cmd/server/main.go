package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/cmd"
	"github.com/kuaizu-team/kuaizu-service/internal/db"
	"github.com/kuaizu-team/kuaizu-service/internal/handler"
	"github.com/kuaizu-team/kuaizu-service/internal/middleware"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/service"
	"github.com/labstack/echo/v4"
	echomiddleware "github.com/labstack/echo/v4/middleware"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func patchMethodOverride() echo.MiddlewareFunc {
	return echomiddleware.MethodOverrideWithConfig(echomiddleware.MethodOverrideConfig{
		Getter: func(c echo.Context) string {
			if c.Request().Header.Get(echo.HeaderXHTTPMethodOverride) == http.MethodPatch {
				return http.MethodPatch
			}
			return ""
		},
	})
}

func main() {
	fmt.Printf("Starting Kuaizu Server %s (Commit: %s, Built at: %s)\n", version, commit, date)
	log.SetFlags(log.Ldate | log.Ltime | log.Llongfile)
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found, using environment variables\n")
	}

	// Initialize Echo
	e := echo.New()
	e.HideBanner = true
<<<<<<< HEAD
	e.Pre(patchMethodOverride())
=======
	e.Pre(echomiddleware.MethodOverride())
>>>>>>> 4962773a9d48e324fbd164cc3eace0ecfd5c0c67

	// Custom colored logger using RequestLoggerWithConfig
	e.Use(cmd.NewRequestLogger())

	e.Use(echomiddleware.Recover())
	e.Use(echomiddleware.CORSWithConfig(cmd.CORSConfig("CORS_ALLOWED_ORIGINS", []string{
		"https://kuaizu.xyz",
		"https://www.kuaizu.xyz",
		"https://dev.darker233.top",
		"http://localhost:3000",
	})))

	// Initialize database connection
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := db.New(ctx)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()
	log.Println("Connected to database")

	// Initialize repository and shared service dependencies
	repo := repository.New(pool)
	startPendingInvitationCleanup(ctx, repo.PendingInvitation)
	deps, err := service.NewDependencies(repo)
	if err != nil {
		log.Fatalf("Failed to initialize service dependencies: %v", err)
	}

	svc := service.New(repo, deps)
	if err := svc.Message.CheckSubscribeDeliverySchema(ctx); err != nil {
		log.Fatalf("Failed to initialize WeChat subscribe delivery: %v", err)
	}
	svc.Message.StartSubscribeDeliveryRecovery(ctx)
	svc.WelcomeEmail.StartPendingRecovery(ctx)
	svc.Payment.StartOrderDeliveryRecovery(ctx)
	server := handler.NewServer(repo, svc)

	// Register API routes with /api/v2 prefix
	apiGroup := e.Group("/api/v2")

	// Add JWT authentication middleware with skipper for public endpoints
	jwtConfig := middleware.DefaultJWTConfig()
	jwtConfig.Skipper = func(c echo.Context) bool {
		path := c.Path()
		method := c.Request().Method

		// Public endpoints that don't require authentication
		publicEndpoints := []string{
			"/api/v2/auth/precheck/wechat",              // WeChat login precheck
			"/api/v2/auth/login/wechat",                 // WeChat login
			"/api/v2/auth/register/phone",               // WeChat phone registration
			"/api/v2/dictionaries/schools",              // School list
			"/api/v2/dictionaries/schools/provinces",    // Province list
			"/api/v2/dictionaries/schools/cities",       // City list
			"/api/v2/dictionaries/schools/districts",    // District list
			"/api/v2/dictionaries/majors",               // Major list
			"/api/v2/email/unsubscribe",                 // Email unsubscribe
			"/api/v2/information/list",                  // Information center list
			"/api/v2/recommendations/projects/featured", // Featured project recommendation
			"/api/v2/recommendations/podcasts",          // Info-center podcast recommendations
			"/api/v2/recommendations/news",              // Info-center news recommendations
			"/api/v2/roadmap",                           // Platform roadmap
		}

		// Keep the legacy timeline public. Personalized school-event views use
		// the same route with a view parameter and require the current user.
		if path == "/api/v2/info-center/events" && strings.TrimSpace(c.QueryParam("view")) == "" {
			return true
		}

		// Check exact matches
		for _, endpoint := range publicEndpoints {
			if path == endpoint {
				return true
			}
		}

		// Public GET endpoints with path parameters
		if method == "GET" {
			if path == "/api/v2/website/team" ||
				path == "/api/v2/website/podcast" ||
				path == "/api/v2/website/projects" {
				return true
			}
			if path == "/api/v2/events" || path == "/api/v2/events/:id" {
				return true
			}
			// /api/v2/projects - list (public)
			if path == "/api/v2/projects" {
				return true
			}
			// /api/v2/talent-profiles - list (public)
			if path == "/api/v2/talent-profiles" {
				return true
			}
		}

		return false
	}
	apiGroup.Use(middleware.JWTAuth(jwtConfig))
	// Block banned / graduated users from all business endpoints.
	// Must run after JWTAuth so "userID" is already set in the Echo context.
	apiGroup.Use(middleware.UserStatusCheck(repo))

	// Register sub-resource routes BEFORE api.RegisterHandlers so that concrete
	// paths are not shadowed by dynamic :id param routes.
	apiGroup.GET("/dictionaries/schools/provinces", server.GetSchoolProvinces)
	apiGroup.GET("/dictionaries/schools/cities", server.GetSchoolCities)
	apiGroup.GET("/dictionaries/schools/districts", server.GetSchoolDistricts)

	// Olive-branch badge endpoints (auth required, registered outside generated code)
	apiGroup.GET("/olive-branches/badge", server.GetOliveBranchBadge)
	apiGroup.POST("/olive-branches/badge/mark-sent-read", server.MarkSentOliveBranchRead)

	// Olive-branch receiver read status
	apiGroup.POST("/olive-branches/received/mark-read", server.MarkReceiverOliveBranchRead)

	apiGroup.POST("/olive-branches/:id/resend", server.ResendOliveBranch)

	// Project-application unread badge endpoints
	apiGroup.GET("/project-applications/my/unread-status", server.GetMyApplicationUnreadStatus)
	apiGroup.POST("/project-applications/my/mark-read", server.MarkMyApplicationsRead)
	apiGroup.GET("/users/me/status-notifications/pending", server.GetMyPendingStatusNotification)
	apiGroup.POST("/users/me/status-notifications/:id/displayed", server.MarkMyStatusNotificationDisplayed)

	// Project-application reviewer read status
	apiGroup.POST("/project-applications/mark-read", server.MarkReviewerApplicationRead)

	// Super-admin invitation feedback
	apiGroup.POST("/invitation/feedback", server.SubmitInvitationFeedback)
	apiGroup.GET("/users/me/pending-invitation", server.GetMyPendingInvitation)
	apiGroup.POST("/users/me/pending-invitation/clear", server.ClearMyPendingInvitation)
	apiGroup.GET("/events", server.ListEvents)
	apiGroup.GET("/website/team", server.ListWebsiteTeam)
	apiGroup.GET("/website/podcast", server.ListWebsitePodcast)
	apiGroup.GET("/website/projects", server.ListWebsiteProjects)
	apiGroup.POST("/events", server.CreateEvent)
	apiGroup.GET("/events/:id", server.GetEvent)
	apiGroup.GET("/info-center/events", server.ListInfoCenterEvents)
	apiGroup.GET("/recommendations/projects", server.ListRecommendationProjects)
	apiGroup.GET("/recommendations/projects/featured", server.GetFeaturedRecommendationProject)
	apiGroup.GET("/recommendations/podcasts", server.ListRecommendationPodcasts)
	apiGroup.GET("/recommendations/news", server.ListRecommendationNews)

	api.RegisterHandlers(apiGroup, server)

	// WeChat Pay callback (no auth required)
	e.POST("/api/v2/payment/wechat/notify", server.WechatPayCallback)

	// Health check endpoint
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(200, map[string]string{"status": "ok"})
	})

	// Stable HTTPS entry used by emails to open the Mini Program customer-service page.
	e.GET("/open/customer-service", server.OpenCustomerService)
	e.HEAD("/open/customer-service", server.OpenCustomerService)

	// Public email entry that redirects to a project-specific Mini Program URL Link.
	e.GET("/api/v2/open/project-detail", server.OpenProjectDetail)
	e.HEAD("/api/v2/open/project-detail", server.OpenProjectDetail)

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s", port)
	if err := cmd.RunHTTPServer(ctx, e, ":"+port); err != nil {
		log.Fatalf("Server stopped with error: %v", err)
	}
	drainCtx, drainCancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer drainCancel()
	if err := svc.EmailPromotion.WaitForSubmissions(drainCtx); err != nil {
		log.Printf("Promotion submissions did not drain cleanly: %v", err)
	}
}

func startPendingInvitationCleanup(ctx context.Context, pending repository.PendingInvitationRepo) {
	const batchSize = 500
	cleanup := func() {
		for {
			deleted, err := pending.DeleteExpired(ctx, time.Now(), batchSize)
			if err != nil {
				if ctx.Err() == nil {
					log.Printf("Failed to clean expired pending invitations: %v", err)
				}
				return
			}
			if deleted < batchSize {
				return
			}
		}
	}

	go func() {
		cleanup()
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cleanup()
			}
		}
	}()
}
