package handler

import (
	"database/sql"
	"errors"
	"strconv"
	"time"

	adminvo "github.com/kuaizu-team/kuaizu-service/internal/admin/vo"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
)

// ListAdmins handles GET /admin/admins
//
// Visibility rules:
//   role=1 → all admins (no filter)
//   role=2 → admins at same school (role=2 and role=3), including self
//   role=3 → only self (frontend hides this page; enforced here as well)
func (s *AdminServer) ListAdmins(ctx echo.Context) error {
	callerRole := adminRole(ctx)
	callerID := currentAdminID(ctx)

	// role=3: return only the caller's own record as a single-item list
	if callerRole == models.AdminRoleSchoolAdmin {
		self, err := s.repo.AdminUser.GetByID(ctx.Request().Context(), callerID)
		if err != nil || self == nil {
			return response.InternalError(ctx, "获取管理员信息失败")
		}
		return response.Success(ctx, map[string]interface{}{
			"list":  []adminvo.AdminUserAccountVO{*adminvo.NewAdminUserAccountVO(self)},
			"total": 1,
		})
	}

	page, _ := strconv.Atoi(ctx.QueryParam("page"))
	size, _ := strconv.Atoi(ctx.QueryParam("pageSize"))
	if size == 0 {
		size, _ = strconv.Atoi(ctx.QueryParam("size"))
	}
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 10
	}

	params := repository.AdminUserListParams{Page: page, Size: size}

	if v := ctx.QueryParam("keyword"); v != "" {
		params.Keyword = &v
	}
	if v := ctx.QueryParam("status"); v != "" {
		st, err := strconv.Atoi(v)
		if err != nil {
			return response.BadRequest(ctx, "invalid status")
		}
		params.Status = &st
	}

	switch callerRole {
	case models.AdminRoleSuperAdmin:
		// role=1: full list; allow optional role filter from query param
		if v := ctx.QueryParam("role"); v != "" {
			r, err := strconv.Atoi(v)
			if err != nil {
				return response.BadRequest(ctx, "invalid role")
			}
			params.Role = &r
		}
	case models.AdminRoleSchoolSuperAdmin:
		// role=2: same school only (includes role=2 and role=3 at that school).
		// role filter is locked to school — no override from query param.
		params.SchoolID = adminSchoolID(ctx)
	}

	admins, total, err := s.repo.AdminUser.List(ctx.Request().Context(), params)
	if err != nil {
		return response.InternalError(ctx, "获取管理员列表失败")
	}

	list := make([]adminvo.AdminUserAccountVO, len(admins))
	for i, a := range admins {
		list[i] = *adminvo.NewAdminUserAccountVO(a)
	}

	return response.Success(ctx, map[string]interface{}{
		"list":  list,
		"total": total,
	})
}

type createAdminRequest struct {
	Username string  `json:"username"`
	Password string  `json:"password"`
	Nickname *string `json:"nickname"`
	Role     int     `json:"role"`
	SchoolID *int    `json:"schoolId"`
	Status   int     `json:"status"`
}

// CreateAdmin handles POST /admin/admins
func (s *AdminServer) CreateAdmin(ctx echo.Context) error {
	callerRole := adminRole(ctx)
	if callerRole == models.AdminRoleSchoolAdmin {
		return response.Forbidden(ctx, "权限不足")
	}

	var req createAdminRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}

	if req.Username == "" || req.Password == "" {
		return response.BadRequest(ctx, "username 和 password 不能为空")
	}
	if req.Role < 1 || req.Role > 3 {
		return response.BadRequest(ctx, "role 必须为 1、2 或 3")
	}

	// 校区超级管理员只能创建 role=3，且 schoolId 只能是自己的学校
	if callerRole == models.AdminRoleSchoolSuperAdmin {
		if req.Role != models.AdminRoleSchoolAdmin {
			return response.Forbidden(ctx, "只能创建校区管理员（role=3）")
		}
		sid := adminSchoolID(ctx)
		if sid == nil || req.SchoolID == nil || *req.SchoolID != *sid {
			return response.Forbidden(ctx, "只能为本校创建管理员")
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return response.InternalError(ctx, "密码加密失败")
	}

	admin := &models.AdminUser{
		Username:     req.Username,
		PasswordHash: string(hash),
		Nickname:     req.Nickname,
		Role:         req.Role,
		SchoolID:     req.SchoolID,
		Status:       req.Status,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := s.repo.AdminUser.Create(ctx.Request().Context(), admin); err != nil {
		if errors.Is(err, repository.ErrDuplicateUsername) {
			return response.BadRequest(ctx, "账号已存在")
		}
		return response.InternalError(ctx, "创建管理员失败")
	}

	// Reload with joined school_name
	created, _ := s.repo.AdminUser.GetByID(ctx.Request().Context(), admin.ID)
	return response.Success(ctx, adminvo.NewAdminUserAccountVO(created))
}

type updateAdminRequest struct {
	Nickname *string `json:"nickname"`
	Password string  `json:"password"` // 空字符串 = 不修改
	Role     *int    `json:"role"`
	SchoolID **int   `json:"schoolId"` // double-pointer: distinguish "not sent" from "set to null"
	Status   *int    `json:"status"`
}

// UpdateAdmin handles PUT /admin/admins/:id
//
// Permission matrix (enforced per canEditAdmin):
//   role=1 → can edit self and any role=2/3; cannot edit other role=1.
//   role=2 → can edit self and same-school role=3; cannot edit role=1/2 (non-self).
//   role=3 → can only edit self.
func (s *AdminServer) UpdateAdmin(ctx echo.Context) error {
	callerRole := adminRole(ctx)
	callerID := currentAdminID(ctx)

	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid admin id")
	}

	target, err := s.repo.AdminUser.GetByID(ctx.Request().Context(), id)
	if err != nil {
		return response.InternalError(ctx, "查询管理员失败")
	}
	if target == nil {
		return response.NotFound(ctx, "管理员不存在")
	}

	if !canEditAdmin(callerRole, callerID, target.Role, target.ID, adminSchoolID(ctx), target.SchoolID) {
		return response.Forbidden(ctx, "权限不足")
	}

	var req updateAdminRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}

	// Apply changes — only overwrite fields that were explicitly sent
	if req.Nickname != nil {
		target.Nickname = req.Nickname
	}
	if req.Role != nil {
		// Callers may not assign a role they couldn't themselves edit.
		// Specifically: role=2 cannot promote a target to role<=2.
		if callerRole == models.AdminRoleSchoolSuperAdmin && *req.Role <= models.AdminRoleSchoolSuperAdmin {
			return response.Forbidden(ctx, "不能将角色提升至校区超级管理员或以上")
		}
		// role=1 callers cannot demote/promote targets to role=1 (another super-admin).
		if callerRole == models.AdminRoleSuperAdmin && *req.Role == models.AdminRoleSuperAdmin && id != callerID {
			return response.Forbidden(ctx, "不能将他人设置为超级管理员")
		}
		target.Role = *req.Role
	}
	if req.SchoolID != nil {
		target.SchoolID = *req.SchoolID
	}
	if req.Status != nil {
		target.Status = *req.Status
	}
	// Password: only update when non-empty
	target.PasswordHash = "" // signal to repository: skip password update
	if req.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return response.InternalError(ctx, "密码加密失败")
		}
		target.PasswordHash = string(hash)
	}

	if err := s.repo.AdminUser.Update(ctx.Request().Context(), target); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return response.NotFound(ctx, "管理员不存在")
		}
		return response.InternalError(ctx, "更新管理员失败")
	}

	updated, _ := s.repo.AdminUser.GetByID(ctx.Request().Context(), id)
	return response.Success(ctx, adminvo.NewAdminUserAccountVO(updated))
}

type updateAdminStatusRequest struct {
	Status int `json:"status"`
}

// UpdateAdminStatus handles PATCH /admin/admins/:id/status
//
// Same permission matrix as UpdateAdmin, plus:
//   Admins may NOT disable their own account (to prevent self-lockout) → 400.
func (s *AdminServer) UpdateAdminStatus(ctx echo.Context) error {
	callerRole := adminRole(ctx)
	callerID := currentAdminID(ctx)

	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid admin id")
	}

	var req updateAdminStatusRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	if req.Status != models.AdminUserStatusDisabled && req.Status != models.AdminUserStatusEnabled {
		return response.BadRequest(ctx, "status 只能为 0（禁用）或 1（启用）")
	}

	// Prevent self-lockout: no one may disable their own account.
	if id == callerID && req.Status == models.AdminUserStatusDisabled {
		return response.BadRequest(ctx, "不能禁用自己的账号")
	}

	target, err := s.repo.AdminUser.GetByID(ctx.Request().Context(), id)
	if err != nil {
		return response.InternalError(ctx, "查询管理员失败")
	}
	if target == nil {
		return response.NotFound(ctx, "管理员不存在")
	}

	if !canEditAdmin(callerRole, callerID, target.Role, target.ID, adminSchoolID(ctx), target.SchoolID) {
		return response.Forbidden(ctx, "权限不足")
	}

	if err := s.repo.AdminUser.UpdateStatus(ctx.Request().Context(), id, req.Status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return response.NotFound(ctx, "管理员不存在")
		}
		return response.InternalError(ctx, "更新状态失败")
	}

	return response.SuccessMessage(ctx, "操作成功")
}

// DeleteAdmin handles DELETE /admin/admins/:id
//
// Same permission matrix as UpdateAdmin.
// Self-deletion is permitted (frontend guides the user to log out afterwards).
func (s *AdminServer) DeleteAdmin(ctx echo.Context) error {
	callerRole := adminRole(ctx)
	callerID := currentAdminID(ctx)

	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid admin id")
	}

	target, err := s.repo.AdminUser.GetByID(ctx.Request().Context(), id)
	if err != nil {
		return response.InternalError(ctx, "查询管理员失败")
	}
	if target == nil {
		return response.NotFound(ctx, "管理员不存在")
	}

	if !canEditAdmin(callerRole, callerID, target.Role, target.ID, adminSchoolID(ctx), target.SchoolID) {
		return response.Forbidden(ctx, "权限不足")
	}

	if err := s.repo.AdminUser.Delete(ctx.Request().Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return response.NotFound(ctx, "管理员不存在")
		}
		return response.InternalError(ctx, "删除管理员失败")
	}

	return response.SuccessMessage(ctx, "操作成功")
}
