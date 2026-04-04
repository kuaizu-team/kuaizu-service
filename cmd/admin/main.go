package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	echomiddleware "github.com/labstack/echo/v4/middleware"
	"github.com/kuaizu-team/kuaizu-service/cmd"
	adminhandler "github.com/kuaizu-team/kuaizu-service/internal/admin/handler"
	adminmw "github.com/kuaizu-team/kuaizu-service/internal/admin/middleware"
	"github.com/kuaizu-team/kuaizu-service/internal/db"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/service"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	fmt.Printf("Starting Kuaizu Admin Server %s (Commit: %s, Built at: %s)\n", version, commit, date)

	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found, using environment variables\n")
	}

	e := echo.New()
	e.HideBanner = true

	e.Use(echomiddleware.Recover())
	e.Use(echomiddleware.CORS())
	e.Use(cmd.NewRequestLogger())

	// Database
	ctx := context.Background()
	pool, err := db.New(ctx)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()
	log.Println("Connected to database")

	repo := repository.New(pool)
	deps, err := service.NewDependencies(repo)
	if err != nil {
		log.Fatalf("Failed to initialize service dependencies: %v", err)
	}

	svc := service.New(repo, deps)
	server := adminhandler.NewAdminServer(repo, svc)

	// Public routes
	e.POST("/admin/auth/login", server.Login)
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	// Protected routes
	adminGroup := e.Group("/admin")
	adminGroup.Use(adminmw.AdminJWTAuth(adminmw.DefaultAdminJWTConfig()))

	adminGroup.GET("/dashboard/stats", server.GetDashboardStats)

	adminGroup.GET("/projects", server.ListProjects)
	adminGroup.GET("/projects/:id", server.GetProject)
	adminGroup.PATCH("/projects/:id", server.ReviewProject)
	adminGroup.PATCH("/talent-profiles/:id", server.ReviewTalentProfile)

	adminGroup.GET("/users", server.ListUsers)
	adminGroup.GET("/users/:id", server.GetUser)
	adminGroup.PATCH("/users/:id/auth", server.ReviewUserAuth)

	adminGroup.GET("/feedbacks", server.ListFeedbacks)
	adminGroup.GET("/feedbacks/:id", server.GetFeedback)
	adminGroup.PATCH("/feedbacks/:id", server.ReplyFeedback)

	port := os.Getenv("ADMIN_PORT")
	if port == "" {
		port = "8081"
	}

	log.Printf("Admin server starting on port %s", port)
	log.Fatal(e.Start(":" + port))
}
