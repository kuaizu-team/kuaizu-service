package handler

import (
	"encoding/json"
	"errors"
	"github.com/kuaizu-team/kuaizu-service/internal/feishusync"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
	"io"
	"net/http"
	"strconv"
)

func (s *AdminServer) SetFeishuSync(sync *feishusync.Service) { s.feishuSync = sync }

func (s *AdminServer) authorizeFeishu(ctx echo.Context, requested int) (int, bool, error) {
	if !canReviewUserTalents(adminRole(ctx)) || currentAdminID(ctx) <= 0 {
		return 0, false, response.Forbidden(ctx, "无飞书同步权限")
	}
	if requested < 0 {
		return 0, false, response.BadRequest(ctx, "学校 ID 无效")
	}
	schoolID, err := feishusync.Authorize(ctx.Request().Context(), s.repo, currentAdminID(ctx), requested)
	if err != nil {
		var permission *feishusync.PermissionError
		if errors.As(err, &permission) {
			return 0, false, response.Forbidden(ctx, permission.Message)
		}
		return 0, false, response.InternalError(ctx, "查询学校同步权限失败")
	}
	school, err := s.repo.School.GetByID(ctx.Request().Context(), schoolID)
	if err != nil {
		return 0, false, response.InternalError(ctx, "查询学校失败")
	}
	if school == nil {
		return 0, false, response.NotFound(ctx, "学校不存在")
	}
	return schoolID, true, nil
}

func (s *AdminServer) StartUserFeishuSync(ctx echo.Context) error {
	if !canReviewUserTalents(adminRole(ctx)) {
		return response.Forbidden(ctx, "无飞书同步权限")
	}
	var req struct {
		SchoolID int `json:"schoolId"`
	}
	decoder := json.NewDecoder(io.LimitReader(ctx.Request().Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		return response.BadRequest(ctx, "请求格式错误，只接受单个 schoolId")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return response.BadRequest(ctx, "请求格式错误")
	}
	schoolID, valid, err := s.authorizeFeishu(ctx, req.SchoolID)
	if !valid {
		return err
	}
	if s.feishuSync == nil {
		return ctx.JSON(http.StatusServiceUnavailable, response.Response{Code: 503, Message: "飞书同步尚未启用，请联系平台管理员完成后端配置"})
	}
	job, err := s.feishuSync.Enqueue(ctx.Request().Context(), currentAdminID(ctx), schoolID)
	if err != nil {
		return response.InternalError(ctx, "创建同步任务失败，请稍后重试或查询当前任务状态")
	}
	ctx.Response().Header().Set("Cache-Control", "no-store")
	return response.Success(ctx, job)
}

// Latest task for an authorized school supports polling and page refresh recovery.
func (s *AdminServer) GetUserFeishuSync(ctx echo.Context) error {
	if !canReviewUserTalents(adminRole(ctx)) {
		return response.Forbidden(ctx, "无飞书同步权限")
	}
	requested := 0
	if value := ctx.QueryParam("schoolId"); value != "" {
		id, err := strconv.Atoi(value)
		if err != nil || id <= 0 {
			return response.BadRequest(ctx, "学校 ID 无效")
		}
		requested = id
	}
	schoolID, valid, err := s.authorizeFeishu(ctx, requested)
	if !valid {
		return err
	}
	if s.feishuSync == nil {
		return ctx.JSON(http.StatusServiceUnavailable, response.Response{Code: 503, Message: "飞书同步尚未启用，请联系平台管理员完成后端配置"})
	}
	job, err := s.feishuSync.Latest(ctx.Request().Context(), schoolID)
	if err != nil {
		return response.InternalError(ctx, "查询同步状态失败")
	}
	ctx.Response().Header().Set("Cache-Control", "no-store")
	// Keep data explicit even when there has been no task yet.
	return ctx.JSON(http.StatusOK, map[string]any{"code": 200, "message": "操作成功", "data": job})
}
