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

	// 校区管理员自动按学校过滤
	if sid := adminSchoolID(ctx); sid != nil {
		params.SchoolID = sid
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

	if v := ctx.QueryParam("schoolId"); v != "" {
		schoolID, err := strconv.Atoi(v)
		if err != nil {
			return response.BadRequest(ctx, "invalid schoolId")
		}
		params.SchoolID = &schoolID
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
	}

	list := make([]adminvo.AdminUserVO, len(result.List))
	for i := range result.List {
		var talentStatus *int
		if status, ok := talentStatusMap[result.List[i].ID]; ok {
			s := status
			talentStatus = &s
		}
		list[i] = *adminvo.NewAdminUserVO(&result.List[i], talentStatus)
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
