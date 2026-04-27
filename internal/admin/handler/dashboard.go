package handler

import (
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
)

type DashboradStatsResponse struct {
	UserCount                 int64 `json:"userCount"`
	ProjectCount              int64 `json:"projectCount"`
	PendingProjectCount       int64 `json:"pendingProjectCount"`
	PendingAuthCount          int64 `json:"pendingAuthCount"`
	PendingFeedbackCount      int64 `json:"pendingFeedbackCount"`
	PendingTalentProfileCount int64 `json:"pendingTalentProfileCount"`
}

// GetDashboardStats handles GET /admin/dashboard/stats
func (s *AdminServer) GetDashboardStats(ctx echo.Context) error {
	db := s.repo.DB()
	rctx := ctx.Request().Context()

	var userCount, projectCount, pendingProjectCount, pendingAuthCount, pendingFeedbackCount, pendingTalentProfileCount int64

	if err := db.QueryRowxContext(rctx, "SELECT COUNT(*) FROM `user`").Scan(&userCount); err != nil {
		return response.InternalError(ctx, "failed to count users")
	}
	if err := db.QueryRowxContext(rctx, "SELECT COUNT(*) FROM project WHERE status != 3").Scan(&projectCount); err != nil {
		return response.InternalError(ctx, "failed to count projects")
	}
	if err := db.QueryRowxContext(rctx, "SELECT COUNT(*) FROM project WHERE status = 0").Scan(&pendingProjectCount); err != nil {
		return response.InternalError(ctx, "failed to count pending projects")
	}
	if err := db.QueryRowxContext(rctx, "SELECT COUNT(*) FROM `user` WHERE auth_status = 0 AND auth_img_url IS NOT NULL").Scan(&pendingAuthCount); err != nil {
		return response.InternalError(ctx, "failed to count pending auths")
	}
	if err := db.QueryRowxContext(rctx, "SELECT COUNT(*) FROM feedback WHERE status = 0").Scan(&pendingFeedbackCount); err != nil {
		return response.InternalError(ctx, "failed to count pending feedbacks")
	}
	if err := db.QueryRowxContext(rctx, "SELECT COUNT(*) FROM talent_profile WHERE status = 2").Scan(&pendingTalentProfileCount); err != nil {
		return response.InternalError(ctx, "failed to count pending talent profiles")
	}

	return response.Success(ctx, DashboradStatsResponse{
		UserCount:                 userCount,
		ProjectCount:              projectCount,
		PendingProjectCount:       pendingProjectCount,
		PendingAuthCount:          pendingAuthCount,
		PendingFeedbackCount:      pendingFeedbackCount,
		PendingTalentProfileCount: pendingTalentProfileCount,
	})
}
