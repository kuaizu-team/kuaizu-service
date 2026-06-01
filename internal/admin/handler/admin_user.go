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

const (
	adminCenterForbiddenMessage     = "校区管理员没有管理员中心权限"
	schoolSuperAdminNoSchoolMessage = "当前校区超级管理员账号未绑定 schoolId，不能操作管理员账号"
)

// ListAdmins handles GET /admin/admins
func (s *AdminServer) ListAdmins(ctx echo.Context) error {
	callerRole := adminRole(ctx)

	if callerRole == models.AdminRoleSchoolAdmin {
		return response.Forbidden(ctx, adminCenterForbiddenMessage)
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
		if v := ctx.QueryParam("role"); v != "" {
			r, err := strconv.Atoi(v)
			if err != nil {
				return response.BadRequest(ctx, "invalid role")
			}
			params.Role = &r
		}
	case models.AdminRoleSchoolSuperAdmin:
		sid := adminSchoolID(ctx)
		if sid == nil {
			return response.Forbidden(ctx, schoolSuperAdminNoSchoolMessage)
		}
		params.SchoolID = sid
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
		return response.Forbidden(ctx, adminCenterForbiddenMessage)
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

	if callerRole == models.AdminRoleSchoolSuperAdmin {
		if req.Role != models.AdminRoleSchoolAdmin {
			return response.Forbidden(ctx, "校区超级管理员只能创建校区管理员")
		}
		sid := adminSchoolID(ctx)
		if sid == nil {
			return response.Forbidden(ctx, schoolSuperAdminNoSchoolMessage)
		}
		req.SchoolID = sid
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

	created, _ := s.repo.AdminUser.GetByID(ctx.Request().Context(), admin.ID)
	return response.Success(ctx, adminvo.NewAdminUserAccountVO(created))
}

type updateAdminRequest struct {
	Nickname *string `json:"nickname"`
	Password string  `json:"password"`
	Role     *int    `json:"role"`
	SchoolID **int   `json:"schoolId"`
	Status   *int    `json:"status"`
}

// UpdateAdmin handles PUT /admin/admins/:id
func (s *AdminServer) UpdateAdmin(ctx echo.Context) error {
	callerRole := adminRole(ctx)
	callerID := currentAdminID(ctx)
	callerSchoolID := adminSchoolID(ctx)

	if callerRole == models.AdminRoleSchoolAdmin {
		return response.Forbidden(ctx, adminCenterForbiddenMessage)
	}
	if callerRole == models.AdminRoleSchoolSuperAdmin && callerSchoolID == nil {
		return response.Forbidden(ctx, schoolSuperAdminNoSchoolMessage)
	}

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

	if !canEditAdmin(callerRole, callerID, target.Role, target.ID, callerSchoolID, target.SchoolID) {
		return response.Forbidden(ctx, "权限不足")
	}

	var req updateAdminRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}

	if req.Nickname != nil {
		target.Nickname = req.Nickname
	}
	if req.Role != nil {
		if callerRole == models.AdminRoleSchoolSuperAdmin && *req.Role <= models.AdminRoleSchoolSuperAdmin {
			return response.Forbidden(ctx, "校区超级管理员不能设置超级管理员或校区超级管理员角色")
		}
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
	if callerRole == models.AdminRoleSchoolSuperAdmin && id != callerID {
		if target.Role != models.AdminRoleSchoolAdmin || !schoolIDsMatch(callerSchoolID, target.SchoolID) {
			return response.Forbidden(ctx, "校区超级管理员只能编辑本校校区管理员")
		}
	}

	target.PasswordHash = ""
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
func (s *AdminServer) UpdateAdminStatus(ctx echo.Context) error {
	callerRole := adminRole(ctx)
	callerID := currentAdminID(ctx)
	callerSchoolID := adminSchoolID(ctx)

	if callerRole == models.AdminRoleSchoolAdmin {
		return response.Forbidden(ctx, adminCenterForbiddenMessage)
	}
	if callerRole == models.AdminRoleSchoolSuperAdmin && callerSchoolID == nil {
		return response.Forbidden(ctx, schoolSuperAdminNoSchoolMessage)
	}

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

	if !canEditAdmin(callerRole, callerID, target.Role, target.ID, callerSchoolID, target.SchoolID) {
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
func (s *AdminServer) DeleteAdmin(ctx echo.Context) error {
	callerRole := adminRole(ctx)
	callerID := currentAdminID(ctx)
	callerSchoolID := adminSchoolID(ctx)

	if callerRole == models.AdminRoleSchoolAdmin {
		return response.Forbidden(ctx, adminCenterForbiddenMessage)
	}
	if callerRole == models.AdminRoleSchoolSuperAdmin && callerSchoolID == nil {
		return response.Forbidden(ctx, schoolSuperAdminNoSchoolMessage)
	}

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

	if !canEditAdmin(callerRole, callerID, target.Role, target.ID, callerSchoolID, target.SchoolID) {
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
