package handler

import (
	"database/sql"
	"strconv"

	"github.com/jmoiron/sqlx"
	adminvo "github.com/kuaizu-team/kuaizu-service/internal/admin/vo"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
)

// ListUsers handles GET /admin/users
func (s *AdminServer) ListUsers(ctx echo.Context) error {
	page, _ := strconv.Atoi(ctx.QueryParam("page"))

	// Accept both "size" and "pageSize" — frontend sends pageSize
	size, _ := strconv.Atoi(ctx.QueryParam("size"))
	if size == 0 {
		size, _ = strconv.Atoi(ctx.QueryParam("pageSize"))
	}

	params := repository.UserListParams{
		Page:                page,
		Size:                size,
		IncludePendingCount: true, // admin list always needs pending count column
	}

	// sortBy / order — unknown values are silently ignored (degraded to default)
	if v := ctx.QueryParam("sortBy"); v != "" {
		params.SortBy = &v
	}
	if v := ctx.QueryParam("order"); v != "" {
		params.Order = &v
	}

	if v := ctx.QueryParam("authStatus"); v != "" {
		status, err := strconv.Atoi(v)
		if err != nil {
			return response.BadRequest(ctx, "invalid authStatus")
		}
		params.AuthStatus = &status
		if params.AuthStatus != nil && *params.AuthStatus == 3 { // 重新映射
			*params.AuthStatus = models.UserAuthStatusNone
			uploaded := true
			params.AuthImgUploaded = &uploaded
		}
	}

	// 仅超级管理员（无学校绑定）可自由指定 schoolId；
	// 校区管理员的 schoolId 参数在下方被强制覆盖，此处仍解析以便做格式校验。
	if v := ctx.QueryParam("schoolId"); v != "" {
		schoolID, err := strconv.Atoi(v)
		if err != nil {
			return response.BadRequest(ctx, "invalid schoolId")
		}
		params.SchoolID = &schoolID
	}

	// 校区管理员强制按本校过滤，放在所有 query param 解析之后，确保不被覆盖。
	if sid := adminSchoolID(ctx); sid != nil {
		params.SchoolID = sid
	}

	if v := ctx.QueryParam("keyword"); v != "" {
		params.Keyword = &v
	}

	if v := ctx.QueryParam("talentProfileStatus"); v != "" {
		status, err := strconv.Atoi(v)
		// -1 表示"从未提交名片"（无名片记录），0/1/2 为正常状态枚举
		if err != nil || (status != -1 && (status < 0 || status > 2)) {
			return response.BadRequest(ctx, "invalid talentProfileStatus, must be -1, 0, 1 or 2")
		}
		params.TalentProfileStatus = &status
	}

	if v := ctx.QueryParam("userId"); v != "" {
		uid, err := strconv.Atoi(v)
		if err != nil {
			return response.BadRequest(ctx, "invalid userId")
		}
		params.UserID = &uid
	}

	if v := ctx.QueryParam("userStatus"); v != "" {
		us, err := strconv.Atoi(v)
		if err != nil || us < 0 || us > 2 {
			return response.BadRequest(ctx, "invalid userStatus, must be 0, 1 or 2")
		}
		params.UserStatus = &us
	}

	result, err := s.svc.User.ListUsers(ctx.Request().Context(), params)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	// Batch-query talent_profile status for all users in this page
	talentStatusMap := make(map[int]int, len(result.List))
	invitationFeedbackStatusMap := make(map[int]string, len(result.List))
	if len(result.List) > 0 {
		userIDs := make([]int, len(result.List))
		for i, u := range result.List {
			userIDs[i] = u.ID
		}
		type tpStatusRow struct {
			UserID int `db:"user_id"`
			Status int `db:"status"`
		}
		q, args, err := sqlx.In(`SELECT user_id, status FROM talent_profile WHERE user_id IN (?)`, userIDs)
		if err == nil {
			q = s.repo.DB().Rebind(q)
			var rows []tpStatusRow
			if err := s.repo.DB().SelectContext(ctx.Request().Context(), &rows, q, args...); err == nil {
				for _, row := range rows {
					talentStatusMap[row.UserID] = row.Status
				}
			}
		}

		// 邀请反馈只对平台超级管理员和校区超级管理员返回。数据库分别记录
		// 初始意向与后续沟通状态，这里归一化为列表展示所需的四种状态。
		if adminRole(ctx) != models.AdminRoleSchoolAdmin {
			type invitationFeedbackRow struct {
				UserID int    `db:"user_id"`
				Status string `db:"status"`
			}
			q, args, err := sqlx.In(`
				SELECT user_id,
					CASE
						WHEN conversation_status IS NOT NULL THEN conversation_status
						WHEN status = 'interested' THEN 'in_progress'
						WHEN status = 'not_interested' THEN 'rejected'
						ELSE 'pending'
					END AS status
				FROM invitation_feedback
				WHERE user_id IN (?)`, userIDs)
			if err == nil {
				q = s.repo.DB().Rebind(q)
				var rows []invitationFeedbackRow
				if err := s.repo.DB().SelectContext(ctx.Request().Context(), &rows, q, args...); err == nil {
					for _, row := range rows {
						invitationFeedbackStatusMap[row.UserID] = row.Status
					}
				}
			}
		}
	}

	list := make([]adminvo.AdminUserVO, len(result.List))
	for i := range result.List {
		var talentStatus *int
		if status, ok := talentStatusMap[result.List[i].ID]; ok {
			s := status
			talentStatus = &s
		}
		list[i] = *adminvo.NewAdminUserVO(&result.List[i], talentStatus)
		if status, ok := invitationFeedbackStatusMap[result.List[i].ID]; ok {
			s := status
			list[i].InvitationFeedbackStatus = &s
		}
	}

	return response.Success(ctx, map[string]interface{}{
		"list":  list,
		"total": result.Total,
		"page":  result.Page,
		"size":  result.Size,
	})
}

// GetUser handles GET /admin/users/:id
func (s *AdminServer) GetUser(ctx echo.Context) error {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid user id")
	}

	user, err := s.svc.User.GetUser(ctx.Request().Context(), id)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	// 校区管理员只能查看本校用户
	if sid := adminSchoolID(ctx); sid != nil {
		if user == nil || user.SchoolID == nil || *user.SchoolID != *sid {
			return response.Forbidden(ctx, "权限不足")
		}
	}

	profile, err := s.repo.TalentProfile.GetByUserID(ctx.Request().Context(), id)
	if err != nil {
		return response.InternalError(ctx, "获取名片信息失败")
	}

	return response.Success(ctx, adminvo.NewAdminUserDetailVO(user, profile))
}

// GetUserActivitySummary returns compact counters used by the admin user detail page.
func (s *AdminServer) GetUserActivitySummary(ctx echo.Context) error {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil || id <= 0 {
		return response.BadRequest(ctx, "invalid user id")
	}

	user, err := s.svc.User.GetUser(ctx.Request().Context(), id)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	if sid := adminSchoolID(ctx); sid != nil && (user == nil || user.SchoolID == nil || *user.SchoolID != *sid) {
		return response.Forbidden(ctx, "权限不足")
	}

	type activitySummary struct {
		ProjectsTotal            int   `db:"projects_total" json:"projectsTotal"`
		ProjectsPending          int   `db:"projects_pending" json:"projectsPending"`
		ProjectsApproved         int   `db:"projects_approved" json:"projectsApproved"`
		ProjectsCompleted        int   `db:"projects_completed" json:"projectsCompleted"`
		ProjectsEnded            int   `db:"projects_ended" json:"projectsEnded"`
		ApplicationsTotal        int   `db:"applications_total" json:"applicationsTotal"`
		ApplicationsPending      int   `db:"applications_pending" json:"applicationsPending"`
		ApplicationsPassed       int   `db:"applications_passed" json:"applicationsPassed"`
		ApplicationsRejected     int   `db:"applications_rejected" json:"applicationsRejected"`
		OliveBranchesTotal       int   `db:"olive_branches_total" json:"oliveBranchesTotal"`
		OliveBranchesPending     int   `db:"olive_branches_pending" json:"oliveBranchesPending"`
		OliveBranchesReadPending int   `db:"olive_branches_read_pending" json:"oliveBranchesReadPending"`
		OrdersTotal              int   `db:"orders_total" json:"ordersTotal"`
		PaidAmount               int64 `db:"paid_amount" json:"paidAmount"`
	}
	var summary activitySummary
	err = s.repo.DB().GetContext(ctx.Request().Context(), &summary, `
		SELECT
		 (SELECT COUNT(*) FROM project WHERE creator_id = ?) projects_total,
		 (SELECT COUNT(*) FROM project WHERE creator_id = ? AND status = 0) projects_pending,
		 (SELECT COUNT(*) FROM project WHERE creator_id = ? AND status = 1) projects_approved,
		 (SELECT COUNT(*) FROM project WHERE creator_id = ? AND status = 3) projects_completed,
		 (SELECT COUNT(*) FROM project WHERE creator_id = ? AND status = 5) projects_ended,
		 (SELECT COUNT(*) FROM project_application WHERE user_id = ?) applications_total,
		 (SELECT COUNT(*) FROM project_application WHERE user_id = ? AND status = 0) applications_pending,
		 (SELECT COUNT(*) FROM project_application WHERE user_id = ? AND status = 1) applications_passed,
		 (SELECT COUNT(*) FROM project_application WHERE user_id = ? AND status = 2) applications_rejected,
		 (SELECT COUNT(*) FROM olive_branch_record WHERE receiver_id = ?) olive_branches_total,
		 (SELECT COUNT(*) FROM olive_branch_record WHERE receiver_id = ? AND status = 0) olive_branches_pending,
		 (SELECT COUNT(*) FROM olive_branch_record WHERE receiver_id = ? AND status = 0 AND is_read = TRUE) olive_branches_read_pending,
		 (SELECT COUNT(*) FROM `+"`order`"+` WHERE user_id = ?) orders_total,
		 (SELECT CAST(COALESCE(ROUND(SUM(actual_paid) * 100), 0) AS SIGNED) FROM `+"`order`"+` WHERE user_id = ? AND status = 1) paid_amount
	`, id, id, id, id, id, id, id, id, id, id, id, id, id, id)
	if err != nil {
		return response.InternalError(ctx, "获取用户活动统计失败")
	}
	if adminRole(ctx) == models.AdminRoleSchoolAdmin {
		summary.OrdersTotal = 0
		summary.PaidAmount = 0
	}
	return response.Success(ctx, summary)
}

func (s *AdminServer) GetUserCollaborationHistory(ctx echo.Context) error {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil || id <= 0 {
		return response.BadRequest(ctx, "invalid user id")
	}
	var list []models.CollaborationScore
	if err := s.repo.DB().SelectContext(ctx.Request().Context(), &list, `
		SELECT cs.id, cs.user_id, cs.project_id, cs.scorer_id, cs.score, cs.created_at,
			p.name AS project_name, u.nickname AS scorer_nickname
		FROM collaboration_score cs
		LEFT JOIN project p ON p.id = cs.project_id
		LEFT JOIN `+"`user`"+` u ON u.id = cs.scorer_id
		WHERE cs.user_id = ?
		ORDER BY cs.created_at DESC, cs.id DESC
	`, id); err != nil {
		return response.InternalError(ctx, "获取协作评分记录失败")
	}
	return response.Success(ctx, list)
}

type updateCollaborationScoreRequest struct {
	Score int `json:"score"`
}

func (s *AdminServer) UpdateUserCollaborationScore(ctx echo.Context) error {
	if adminRole(ctx) != models.AdminRoleSuperAdmin {
		return response.Forbidden(ctx, "权限不足")
	}
	userID, err := strconv.Atoi(ctx.Param("id"))
	if err != nil || userID <= 0 {
		return response.BadRequest(ctx, "invalid user id")
	}
	scoreID, err := strconv.Atoi(ctx.Param("scoreId"))
	if err != nil || scoreID <= 0 {
		return response.BadRequest(ctx, "invalid score id")
	}
	var req updateCollaborationScoreRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	if req.Score < 0 || req.Score > 100 {
		return response.BadRequest(ctx, "score must be between 0 and 100")
	}

	tx, err := s.repo.DB().BeginTxx(ctx.Request().Context(), nil)
	if err != nil {
		return response.InternalError(ctx, "开启事务失败")
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx.Request().Context(), "UPDATE collaboration_score SET score=? WHERE id=? AND user_id=?", req.Score, scoreID, userID)
	if err != nil {
		return response.InternalError(ctx, "更新评分失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return response.NotFound(ctx, "评分记录不存在")
	}
	if _, err := tx.ExecContext(ctx.Request().Context(), `UPDATE `+"`user`"+` SET collaboration_score=(
		SELECT avg_score FROM (SELECT COALESCE(AVG(score), 100) AS avg_score FROM collaboration_score WHERE user_id=?) t
	) WHERE id=?`, userID, userID); err != nil {
		return response.InternalError(ctx, "更新协作指数失败")
	}
	if err := tx.Commit(); err != nil {
		return response.InternalError(ctx, "提交事务失败")
	}
	return response.Success(ctx, map[string]interface{}{"ok": true})
}

// ListUserApplications handles GET /admin/users/:id/applications
func (s *AdminServer) ListUserApplications(ctx echo.Context) error {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid user id")
	}

	page, _ := strconv.Atoi(ctx.QueryParam("page"))
	size, _ := strconv.Atoi(ctx.QueryParam("size"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 10
	}

	params := repository.ApplicationListParams{
		UserID: &id,
		Page:   page,
		Size:   size,
	}

	applications, total, err := s.repo.Application.List(ctx.Request().Context(), params)
	if err != nil {
		return response.InternalError(ctx, "获取投递记录失败")
	}

	list := make([]adminvo.AdminApplicationVO, len(applications))
	for i := range applications {
		list[i] = *adminvo.NewAdminApplicationVO(&applications[i])
	}

	return response.Success(ctx, map[string]interface{}{
		"list":  list,
		"total": total,
	})
}

// ListUserOliveBranches handles GET /admin/users/:id/olive-branches
func (s *AdminServer) ListUserOliveBranches(ctx echo.Context) error {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid user id")
	}

	page, _ := strconv.Atoi(ctx.QueryParam("page"))
	size, _ := strconv.Atoi(ctx.QueryParam("size"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 10
	}

	params := repository.OliveBranchListParams{
		ReceiverID: id,
		Page:       page,
		Size:       size,
	}

	branches, total, err := s.repo.OliveBranch.ListByReceiverID(ctx.Request().Context(), params)
	if err != nil {
		return response.InternalError(ctx, "获取橄榄枝记录失败")
	}

	list := make([]adminvo.AdminOliveBranchVO, len(branches))
	for i := range branches {
		list[i] = *adminvo.NewAdminOliveBranchVO(&branches[i])
	}

	return response.Success(ctx, map[string]interface{}{
		"list":  list,
		"total": total,
	})
}

// ListUserOrders handles GET /admin/users/:id/orders.
func (s *AdminServer) ListUserOrders(ctx echo.Context) error {
	if adminRole(ctx) == models.AdminRoleSchoolAdmin {
		return response.Forbidden(ctx, "permission denied")
	}

	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid user id")
	}

	page, _ := strconv.Atoi(ctx.QueryParam("page"))
	size, _ := strconv.Atoi(ctx.QueryParam("size"))
	if size == 0 {
		size, _ = strconv.Atoi(ctx.QueryParam("pageSize"))
	}
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 10
	}

	params := repository.AdminOrderListParams{
		UserID: &id,
		Page:   page,
		Size:   size,
	}
	if sid := adminSchoolID(ctx); sid != nil {
		params.SchoolID = sid
	}

	orders, total, err := s.repo.Order.AdminList(ctx.Request().Context(), params)
	if err != nil {
		return response.InternalError(ctx, "get user orders failed")
	}

	list := make([]adminvo.AdminOrderVO, len(orders))
	for i, o := range orders {
		list[i] = *adminvo.NewAdminOrderVO(o)
	}

	return response.Success(ctx, map[string]interface{}{
		"list":  list,
		"total": total,
	})
}

type updateUserStatusRequest struct {
	Status    int     `json:"status"`
	BanReason *string `json:"banReason"`
}

// UpdateUserStatus handles PUT /admin/users/:id/status
func (s *AdminServer) UpdateUserStatus(ctx echo.Context) error {
	// role=3 (校区管理员) 无权操作
	if adminRole(ctx) == models.AdminRoleSchoolAdmin {
		return response.Forbidden(ctx, "权限不足")
	}

	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid user id")
	}

	var req updateUserStatusRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	if req.Status < 0 || req.Status > 2 {
		return response.BadRequest(ctx, "invalid status, must be 0, 1 or 2")
	}

	// role=2 只能操作本校用户
	if sid := adminSchoolID(ctx); sid != nil {
		user, err := s.svc.User.GetUser(ctx.Request().Context(), id)
		if err != nil {
			return mapServiceError(ctx, err)
		}
		if user == nil || user.SchoolID == nil || *user.SchoolID != *sid {
			return response.Forbidden(ctx, "权限不足")
		}
	}

	if err := s.repo.User.UpdateUserStatus(ctx.Request().Context(), id, req.Status, req.BanReason); err != nil {
		if err == sql.ErrNoRows {
			return response.NotFound(ctx, "用户不存在")
		}
		return response.InternalError(ctx, "操作失败")
	}

	return response.SuccessMessage(ctx, "操作成功")
}

type reviewAuthRequest struct {
	AuthStatus int `json:"authStatus"`
}

// ReviewUserAuth handles PATCH /admin/users/:id/auth
func (s *AdminServer) ReviewUserAuth(ctx echo.Context) error {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid user id")
	}

	// 校区管理员只能操作本校用户
	if sid := adminSchoolID(ctx); sid != nil {
		user, err := s.svc.User.GetUser(ctx.Request().Context(), id)
		if err != nil {
			return mapServiceError(ctx, err)
		}
		if user == nil || user.SchoolID == nil || *user.SchoolID != *sid {
			return response.Forbidden(ctx, "权限不足")
		}
	}

	var req reviewAuthRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}

	if req.AuthStatus != models.UserAuthStatusPassed && req.AuthStatus != models.UserAuthStatusFailed {
		return response.BadRequest(ctx, "invalid authStatus, must be 1 (approve) or 2 (reject)")
	}

	if err := s.svc.User.ReviewUserAuth(ctx.Request().Context(), id, req.AuthStatus); err != nil {
		return mapServiceError(ctx, err)
	}

	return response.SuccessMessage(ctx, "操作成功")
}
