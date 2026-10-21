package handler

import (
	"fmt"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
)

type userSelectionRequest struct {
	UserIDs []int `json:"userIds"`
}

func validateUserIDs(ids []int, limit int) error {
	if len(ids) == 0 || len(ids) > limit {
		return fmt.Errorf("请选择 1 至 %d 个用户", limit)
	}
	seen := make(map[int]bool, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return fmt.Errorf("用户 ID 必须为正整数且不能重复")
		}
		seen[id] = true
	}
	return nil
}

func (s *AdminServer) BatchApproveUsers(ctx echo.Context) error {
	if !canReviewUserTalents(adminRole(ctx)) {
		return response.Forbidden(ctx, "无批量审核权限")
	}
	var req userSelectionRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "请求格式错误")
	}
	if err := validateUserIDs(req.UserIDs, 100); err != nil {
		return response.BadRequest(ctx, err.Error())
	}
	schools, err := s.adminSchoolIDs(ctx)
	if err != nil {
		return response.InternalError(ctx, "查询学校权限失败")
	}
	result, err := s.svc.TalentProfile.BatchApproveUsers(ctx.Request().Context(), req.UserIDs, schools)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return response.Success(ctx, result)
}

func canReviewUserTalents(role int) bool {
	return role == models.AdminRoleSuperAdmin || role == models.AdminRoleSchoolSuperAdmin || role == models.AdminRoleSchoolAdmin
}
