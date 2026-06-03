package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	"github.com/kuaizu-team/kuaizu-service/cmd"
	adminhandler "github.com/kuaizu-team/kuaizu-service/internal/admin/handler"
	adminmw "github.com/kuaizu-team/kuaizu-service/internal/admin/middleware"
	"github.com/kuaizu-team/kuaizu-service/internal/db"
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
	adminGroup.GET("/stats/revenue", server.GetRevenueStats)
	adminGroup.GET("/stats/registrations", server.GetRegistrationStats)
	adminGroup.GET("/stats/activations", server.GetActivationStats)

	adminGroup.GET("/projects", server.ListProjects)
	adminGroup.GET("/projects/:id", server.GetProject)
	adminGroup.PATCH("/projects/:id", server.ReviewProject)
	adminGroup.PATCH("/projects/:id/takedown", server.TakedownProject)
	adminGroup.GET("/projects/:id/applications", server.ListProjectApplications)
	adminGroup.GET("/projects/:id/olive-branches", server.ListProjectOliveBranches)
	adminGroup.PATCH("/talent-profiles/:id", server.ReviewTalentProfile)
	adminGroup.PATCH("/talent-profiles/:id/takedown", server.TakedownTalentProfile)

	adminGroup.GET("/users", server.ListUsers)
	adminGroup.GET("/users/:id", server.GetUser)
	adminGroup.PATCH("/users/:id/auth", server.ReviewUserAuth)
	adminGroup.PUT("/users/:id/status", server.UpdateUserStatus)
	adminGroup.GET("/users/:id/applications", server.ListUserApplications)
	adminGroup.GET("/users/:id/olive-branches", server.ListUserOliveBranches)

	adminGroup.GET("/feedbacks", server.ListFeedbacks)
	adminGroup.GET("/feedbacks/:id", server.GetFeedback)
	adminGroup.PATCH("/feedbacks/:id", server.ReplyFeedback)

	adminGroup.GET("/information", server.ListInformation)
	adminGroup.GET("/information/:id", server.GetInformation)
	adminGroup.POST("/information", server.CreateInformation)
	adminGroup.PUT("/information/:id", server.UpdateInformation)
	adminGroup.DELETE("/information/:id", server.DeleteInformation)

	adminGroup.GET("/orders", server.ListOrders)
	adminGroup.POST("/orders/:id/refund/apply", server.ApplyOrderRefund)
	adminGroup.PATCH("/orders/:id/refund", server.ReviewOrderRefund)
	adminGroup.GET("/orders/:id", server.GetOrder)

	adminGroup.GET("/admins", server.ListAdmins)
	adminGroup.POST("/admins", server.CreateAdmin)
	adminGroup.GET("/admins/:id", server.GetAdmin)
	adminGroup.PATCH("/admins/:id/finance-remark", server.UpdateAdminFinanceRemark)
	adminGroup.POST("/admins/:id/settle", server.SettleAdminOrders)
	adminGroup.PUT("/admins/:id", server.UpdateAdmin)
	adminGroup.PATCH("/admins/:id/status", server.UpdateAdminStatus)
	adminGroup.DELETE("/admins/:id", server.DeleteAdmin)

	adminGroup.GET("/schools", server.ListSchools)

	port := os.Getenv("ADMIN_PORT")
	if port == "" {
		port = "8081"
	}

	log.Printf("Admin server starting on port %s", port)
	log.Fatal(e.Start(":" + port))
}
