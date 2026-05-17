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
	sid := adminSchoolID(ctx) // nil for super admin

	var userCount, projectCount, pendingProjectCount, pendingAuthCount, pendingFeedbackCount, pendingTalentProfileCount int64

	if sid == nil {
		// Super admin — global counts
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
	} else {
		// Campus admin — counts scoped to their school
		schoolID := *sid
		if err := db.QueryRowxContext(rctx,
			"SELECT COUNT(*) FROM `user` WHERE school_id = ?", schoolID).Scan(&userCount); err != nil {
			return response.InternalError(ctx, "failed to count users")
		}
		if err := db.QueryRowxContext(rctx,
			"SELECT COUNT(*) FROM project WHERE status != 3 AND school_id = ?", schoolID).Scan(&projectCount); err != nil {
			return response.InternalError(ctx, "failed to count projects")
		}
		if err := db.QueryRowxContext(rctx,
			"SELECT COUNT(*) FROM project WHERE status = 0 AND school_id = ?", schoolID).Scan(&pendingProjectCount); err != nil {
			return response.InternalError(ctx, "failed to count pending projects")
		}
		if err := db.QueryRowxContext(rctx, `
			SELECT COUNT(*) FROM `+"`user`"+`
			WHERE auth_status = 0 AND auth_img_url IS NOT NULL AND school_id = ?`,
			schoolID).Scan(&pendingAuthCount); err != nil {
			return response.InternalError(ctx, "failed to count pending auths")
		}
		if err := db.QueryRowxContext(rctx, `
			SELECT COUNT(*) FROM feedback f
			JOIN `+"`user`"+` u ON f.user_id = u.id
			WHERE f.status = 0 AND u.school_id = ?`,
			schoolID).Scan(&pendingFeedbackCount); err != nil {
			return response.InternalError(ctx, "failed to count pending feedbacks")
		}
		if err := db.QueryRowxContext(rctx, `
			SELECT COUNT(*) FROM talent_profile tp
			JOIN `+"`user`"+` u ON tp.user_id = u.id
			WHERE tp.status = 2 AND u.school_id = ?`,
			schoolID).Scan(&pendingTalentProfileCount); err != nil {
			return response.InternalError(ctx, "failed to count pending talent profiles")
		}
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
