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
	adminGroup.GET("/auth/me", server.GetCurrentAdmin)

	adminGroup.GET("/dashboard/stats", server.GetDashboardStats)
	adminGroup.GET("/stats/revenue", server.GetRevenueStats)
	adminGroup.GET("/stats/registrations", server.GetRegistrationStats)
	adminGroup.GET("/stats/activations", server.GetActivationStats)

	adminGroup.GET("/projects", server.ListProjects)
	adminGroup.GET("/projects/:id", server.GetProject)
	adminGroup.PATCH("/projects/:id", server.ReviewProject)
	adminGroup.PATCH("/projects/:id/takedown", server.TakedownProject)
	adminGroup.PATCH("/projects/:id/restore", server.RestoreProject)
	adminGroup.POST("/projects/:id/milestones", server.CreateProjectMilestone)
	adminGroup.PATCH("/projects/:id/members/:memberId/role", server.UpdateProjectMemberRole)
	adminGroup.PUT("/projects/:id/events", server.ReplaceProjectEvents)
	adminGroup.PATCH("/projects/:id/admin-note", server.UpdateProjectAdminNote)
	adminGroup.PATCH("/projects/:id/status", server.UpdateProjectLifecycleStatus)
	adminGroup.DELETE("/projects/:id/permanent", server.PermanentlyDeleteProject)
	adminGroup.GET("/projects/:id/applications", server.ListProjectApplications)
	adminGroup.GET("/projects/:id/olive-branches", server.ListProjectOliveBranches)
	adminGroup.GET("/projects/:id/activity-summary", server.GetProjectActivitySummary)
	adminGroup.PATCH("/talent-profiles/:id", server.ReviewTalentProfile)
	adminGroup.PATCH("/talent-profiles/:id/takedown", server.TakedownTalentProfile)

	adminGroup.GET("/users", server.ListUsers)
	adminGroup.GET("/users/:id/orders", server.ListUserOrders)
	adminGroup.GET("/users/:id/invitation-status", server.GetUserInvitationStatus)
	adminGroup.PUT("/users/:id/invitation/conversation-status", server.UpdateUserInvitationConversationStatus)
	adminGroup.GET("/users/:id/ratings", server.ListUserProjectRatings)
	adminGroup.PUT("/ratings/:id", server.UpdateProjectRating)
	adminGroup.GET("/users/:id/collaboration-history", server.GetUserCollaborationHistory)
	adminGroup.GET("/users/:id", server.GetUser)
	adminGroup.GET("/users/:id/activity-summary", server.GetUserActivitySummary)
	adminGroup.PATCH("/users/:id/auth", server.ReviewUserAuth)
	adminGroup.PUT("/users/:id/status", server.UpdateUserStatus)
	adminGroup.PUT("/users/:id/competition-group", server.UpdateUserCompetitionGroup)
	adminGroup.GET("/users/:id/applications", server.ListUserApplications)
	adminGroup.GET("/users/:id/olive-branches", server.ListUserOliveBranches)

	adminGroup.POST("/sms/send", server.SendAdminSms)
	adminGroup.GET("/sms/send-count", server.CountAdminSms)

	adminGroup.GET("/feedbacks", server.ListFeedbacks)
	adminGroup.GET("/feedbacks/:id", server.GetFeedback)
	adminGroup.PATCH("/feedbacks/:id", server.ReplyFeedback)

	adminGroup.GET("/events", server.ListEvents)
	adminGroup.POST("/events", server.CreateEvent)
	adminGroup.PUT("/events/:id", server.UpdateEvent)
	adminGroup.DELETE("/events/:id", server.DeleteEvent)
	adminGroup.POST("/events/:id/merge", server.MergeEvent)

	adminGroup.GET("/recommendations/projects", server.ListProjectRecommendations)
	adminGroup.POST("/recommendations/projects", server.CreateProjectRecommendation)
	adminGroup.PUT("/recommendations/projects/:id", server.UpdateProjectRecommendation)
	adminGroup.DELETE("/recommendations/projects/:id", server.DeleteProjectRecommendation)
	adminGroup.GET("/recommendations/podcasts", server.ListPodcastRecommendations)
	adminGroup.GET("/recommendations/podcasts/:id", server.GetPodcastRecommendation)
	adminGroup.POST("/recommendations/podcasts", server.CreatePodcastRecommendation)
	adminGroup.PUT("/recommendations/podcasts/:id", server.UpdatePodcastRecommendation)
	adminGroup.DELETE("/recommendations/podcasts/:id", server.DeletePodcastRecommendation)
	adminGroup.GET("/recommendations/news", server.ListNewsRecommendations)
	adminGroup.GET("/recommendations/news/:id", server.GetNewsRecommendation)
	adminGroup.POST("/recommendations/news", server.CreateNewsRecommendation)
	adminGroup.PUT("/recommendations/news/:id", server.UpdateNewsRecommendation)
	adminGroup.DELETE("/recommendations/news/:id", server.DeleteNewsRecommendation)
	adminGroup.GET("/information", server.ListInformation)
	adminGroup.GET("/information/:id", server.GetInformation)
	adminGroup.POST("/information", server.CreateInformation)
	adminGroup.PUT("/information/:id", server.UpdateInformation)
	adminGroup.DELETE("/information/:id", server.DeleteInformation)

	adminGroup.GET("/roadmap", server.ListRoadmaps)
	adminGroup.GET("/roadmap/:id", server.GetRoadmap)
	adminGroup.POST("/roadmap", server.CreateRoadmap)
	adminGroup.PUT("/roadmap/:id", server.UpdateRoadmap)
	adminGroup.DELETE("/roadmap/:id", server.DeleteRoadmap)
	adminGroup.POST("/send-version-update", server.SendVersionUpdate)

	adminGroup.GET("/orders", server.ListOrders)
	adminGroup.POST("/orders/:id/refund/apply", server.ApplyOrderRefund)
	adminGroup.PATCH("/orders/:id/refund/reject", server.RejectOrderRefund)
	adminGroup.POST("/orders/:id/refund/reject", server.RejectOrderRefund)
	adminGroup.PATCH("/orders/:id/refund/withdraw", server.WithdrawOrderRefund)
	adminGroup.POST("/orders/:id/refund/withdraw", server.WithdrawOrderRefund)
	adminGroup.PATCH("/orders/:id/refund", server.ReviewOrderRefund)
	adminGroup.GET("/orders/:id", server.GetOrder)

	adminGroup.GET("/admins", server.ListAdmins)
	adminGroup.POST("/admins", server.CreateAdmin)
	adminGroup.GET("/admins/:id", server.GetAdmin)
	adminGroup.PATCH("/admins/:id/finance-remark", server.UpdateAdminFinanceRemark)
	adminGroup.PUT("/admins/:id/commission-rate", server.UpdateAdminCommissionRate)
	adminGroup.POST("/admins/:id/settle", server.SettleAdminOrders)
	adminGroup.POST("/delegate", server.DelegateAdminSchool)
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
