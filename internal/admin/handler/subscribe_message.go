package handler

import (
	"strconv"
	"strings"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
)

type updateSubscribeTemplateRequest struct {
	Enabled *bool `json:"enabled"`
}

func (s *AdminServer) GetSubscribeMessageOverview(ctx echo.Context) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}
	limit := 50
	if value := ctx.QueryParam("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 || parsed > 200 {
			return response.BadRequest(ctx, "limit must be between 1 and 200")
		}
		limit = parsed
	}
	templates, err := s.repo.MsgTemplate.ListAll(ctx.Request().Context())
	if err != nil {
		return response.InternalError(ctx, "list subscribe templates failed")
	}
	deliveries, err := s.repo.WxSubscribeDelivery.ListRecent(ctx.Request().Context(), limit)
	if err != nil {
		return response.InternalError(ctx, "list subscribe deliveries failed")
	}
	counts, err := s.repo.WxSubscribeDelivery.CountByStatusSince(ctx.Request().Context(), time.Now().Add(-7*24*time.Hour))
	if err != nil {
		return response.InternalError(ctx, "count subscribe deliveries failed")
	}
	return response.Success(ctx, map[string]interface{}{
		"templates":  templates,
		"deliveries": deliveries,
		"last7Days":  counts,
	})
}

func (s *AdminServer) UpdateSubscribeMessageTemplate(ctx echo.Context) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}
	bizKey := strings.TrimSpace(ctx.Param("bizKey"))
	if !strings.HasPrefix(bizKey, "MSG_") {
		return response.BadRequest(ctx, "invalid subscribe message biz key")
	}
	var req updateSubscribeTemplateRequest
	if err := ctx.Bind(&req); err != nil || req.Enabled == nil {
		return response.BadRequest(ctx, "enabled is required")
	}
	updated, err := s.repo.MsgTemplate.UpdateEnabled(ctx.Request().Context(), bizKey, *req.Enabled)
	if err != nil {
		return response.InternalError(ctx, "update subscribe template failed")
	}
	if !updated {
		return response.NotFound(ctx, "subscribe template not found")
	}
	return response.Success(ctx, map[string]interface{}{"bizKey": bizKey, "enabled": *req.Enabled})
}
