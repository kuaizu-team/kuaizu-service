package handler

import (
	"strconv"

	adminvo "github.com/kuaizu-team/kuaizu-service/internal/admin/vo"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
)

// ListUsers handles GET /admin/users
func (s *AdminServer) ListUsers(ctx echo.Context) error {
	page, _ := strconv.Atoi(ctx.QueryParam("page"))
	size, _ := strconv.Atoi(ctx.QueryParam("size"))

	params := repository.UserListParams{
		Page: page,
		Size: size,
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

	result, err := s.svc.User.ListUsers(ctx.Request().Context(), params)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	list := make([]adminvo.AdminUserVO, len(result.List))
	for i := range result.List {
		list[i] = *adminvo.NewAdminUserVO(&result.List[i])
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

	profile, err := s.repo.TalentProfile.GetByUserID(ctx.Request().Context(), id)
	if err != nil {
		return response.InternalError(ctx, "获取名片信息失败")
	}

	return response.Success(ctx, adminvo.NewAdminUserDetailVO(user, profile))
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
