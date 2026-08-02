package handler

import (
	"strconv"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
)

type DashboradStatsResponse struct {
	UserCount                 int64 `json:"userCount"`
	ProjectCount              int64 `json:"projectCount"`
	PendingProjectCount       int64 `json:"pendingProjectCount"`
	DeletingProjectCount      int64 `json:"deletingProjectCount"`
	PendingAuthCount          int64 `json:"pendingAuthCount"`
	PendingFeedbackCount      int64 `json:"pendingFeedbackCount"`
	PendingTalentProfileCount int64 `json:"pendingTalentProfileCount"`
}

// GetDashboardStats handles GET /admin/dashboard/stats
func (s *AdminServer) GetDashboardStats(ctx echo.Context) error {
	db := s.repo.DB()
	rctx := ctx.Request().Context()
	sid := adminSchoolID(ctx) // nil for super admin

	var userCount, projectCount, pendingProjectCount, deletingProjectCount, pendingAuthCount, pendingFeedbackCount, pendingTalentProfileCount int64
	if adminRole(ctx) == models.AdminRoleEventManager {
		eventID, err := s.eventIDForManager(ctx)
		if err != nil {
			return err
		}
		if err := db.QueryRowxContext(rctx, `SELECT COUNT(*), COALESCE(SUM(p.status=0),0), COALESCE(SUM(p.status=4),0) FROM project p JOIN project_event pe ON pe.project_id=p.id WHERE pe.event_id=?`, eventID).Scan(&projectCount, &pendingProjectCount, &deletingProjectCount); err != nil {
			return response.InternalError(ctx, "failed to count event projects")
		}
		return response.Success(ctx, DashboradStatsResponse{ProjectCount: projectCount, PendingProjectCount: pendingProjectCount, DeletingProjectCount: deletingProjectCount})
	}
	if adminRole(ctx) == models.AdminRoleSchoolSuperAdmin {
		schoolIDs, err := s.adminSchoolIDs(ctx)
		if err != nil {
			return response.InternalError(ctx, "查询学校权限失败")
		}
		if len(schoolIDs) == 0 {
			return response.Success(ctx, DashboradStatsResponse{})
		}
		query, args, err := sqlx.In(`SELECT
			(SELECT COUNT(*) FROM `+"`user`"+` WHERE school_id IN (?)),
			(SELECT COUNT(*) FROM project WHERE status<>3 AND school_id IN (?)),
			(SELECT COUNT(*) FROM project WHERE status=0 AND school_id IN (?)),
			(SELECT COUNT(*) FROM project WHERE status=4 AND school_id IN (?)),
			(SELECT COUNT(*) FROM `+"`user`"+` WHERE auth_status=0 AND auth_img_url IS NOT NULL AND school_id IN (?)),
			(SELECT COUNT(*) FROM feedback f JOIN `+"`user`"+` u ON u.id=f.user_id WHERE f.status=0 AND u.school_id IN (?)),
			(SELECT COUNT(*) FROM talent_profile tp JOIN `+"`user`"+` u ON u.id=tp.user_id WHERE tp.status=2 AND u.school_id IN (?))`,
			schoolIDs, schoolIDs, schoolIDs, schoolIDs, schoolIDs, schoolIDs, schoolIDs)
		if err != nil {
			return response.InternalError(ctx, "failed to build dashboard query")
		}
		if err := db.QueryRowxContext(rctx, db.Rebind(query), args...).Scan(
			&userCount, &projectCount, &pendingProjectCount, &deletingProjectCount,
			&pendingAuthCount, &pendingFeedbackCount, &pendingTalentProfileCount,
		); err != nil {
			return response.InternalError(ctx, "failed to load dashboard stats")
		}
		return response.Success(ctx, DashboradStatsResponse{
			UserCount: userCount, ProjectCount: projectCount, PendingProjectCount: pendingProjectCount,
			DeletingProjectCount: deletingProjectCount, PendingAuthCount: pendingAuthCount,
			PendingFeedbackCount: pendingFeedbackCount, PendingTalentProfileCount: pendingTalentProfileCount,
		})
	}

	if sid == nil {
		// Super admin — global counts
		if err := db.QueryRowxContext(rctx, `SELECT
			(SELECT COUNT(*) FROM `+"`user`"+`),
			(SELECT COUNT(*) FROM project WHERE status<>3),
			(SELECT COUNT(*) FROM project WHERE status=0),
			(SELECT COUNT(*) FROM project WHERE status=4),
			(SELECT COUNT(*) FROM `+"`user`"+` WHERE auth_status=0 AND auth_img_url IS NOT NULL),
			(SELECT COUNT(*) FROM feedback WHERE status=0),
			(SELECT COUNT(*) FROM talent_profile WHERE status=2)`).Scan(
			&userCount, &projectCount, &pendingProjectCount, &deletingProjectCount,
			&pendingAuthCount, &pendingFeedbackCount, &pendingTalentProfileCount,
		); err != nil {
			return response.InternalError(ctx, "failed to load dashboard stats")
		}
	} else {
		// Campus admin — counts scoped to their school
		schoolID := *sid
		if err := db.QueryRowxContext(rctx, `SELECT
			(SELECT COUNT(*) FROM `+"`user`"+` WHERE school_id=?),
			(SELECT COUNT(*) FROM project WHERE status<>3 AND school_id=?),
			(SELECT COUNT(*) FROM project WHERE status=0 AND school_id=?),
			(SELECT COUNT(*) FROM project WHERE status=4 AND school_id=?),
			(SELECT COUNT(*) FROM `+"`user`"+` WHERE auth_status=0 AND auth_img_url IS NOT NULL AND school_id=?),
			(SELECT COUNT(*) FROM feedback f JOIN `+"`user`"+` u ON u.id=f.user_id WHERE f.status=0 AND u.school_id=?),
			(SELECT COUNT(*) FROM talent_profile tp JOIN `+"`user`"+` u ON u.id=tp.user_id WHERE tp.status=2 AND u.school_id=?)`,
			schoolID, schoolID, schoolID, schoolID, schoolID, schoolID, schoolID,
		).Scan(
			&userCount, &projectCount, &pendingProjectCount, &deletingProjectCount,
			&pendingAuthCount, &pendingFeedbackCount, &pendingTalentProfileCount,
		); err != nil {
			return response.InternalError(ctx, "failed to load dashboard stats")
		}
	}

	return response.Success(ctx, DashboradStatsResponse{
		UserCount:                 userCount,
		ProjectCount:              projectCount,
		PendingProjectCount:       pendingProjectCount,
		DeletingProjectCount:      deletingProjectCount,
		PendingAuthCount:          pendingAuthCount,
		PendingFeedbackCount:      pendingFeedbackCount,
		PendingTalentProfileCount: pendingTalentProfileCount,
	})
}

func (s *AdminServer) GetRevenueStats(ctx echo.Context) error {
	role := adminRole(ctx)
	if role == models.AdminRoleSchoolAdmin {
		return response.Forbidden(ctx, "权限不足")
	}

	if role == models.AdminRoleSchoolSuperAdmin {
		schoolIDs, err := s.adminSchoolIDs(ctx)
		if err != nil {
			return response.InternalError(ctx, "查询学校权限失败")
		}
		if len(schoolIDs) == 0 {
			return response.Forbidden(ctx, "当前管理员未绑定学校")
		}
		stats, err := s.repo.Order.RevenueStatsForSchools(ctx.Request().Context(), schoolIDs)
		if err != nil {
			return response.InternalError(ctx, "获取营收统计失败")
		}
		adminStats, err := s.repo.Order.AdminRevenueStats(ctx.Request().Context(), currentAdminID(ctx))
		if err != nil {
			return response.InternalError(ctx, "获取待结算统计失败")
		}
		stats.PendingSettlementAmount = adminStats.PendingSettlementAmount
		return response.Success(ctx, stats)
	}

	var schoolID *int
	if role == models.AdminRoleSchoolSuperAdmin {
		schoolID = adminSchoolID(ctx)
		if schoolID == nil {
			return response.Forbidden(ctx, "当前管理员未绑定学校")
		}
	} else if v := ctx.QueryParam("schoolId"); v != "" {
		id, err := strconv.Atoi(v)
		if err != nil {
			return response.BadRequest(ctx, "invalid schoolId")
		}
		schoolID = &id
	}

	stats, err := s.repo.Order.RevenueStats(ctx.Request().Context(), schoolID)
	if err != nil {
		return response.InternalError(ctx, "获取营收统计失败")
	}
	return response.Success(ctx, stats)
}
