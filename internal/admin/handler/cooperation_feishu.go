package handler

import (
	"github.com/kuaizu-team/kuaizu-service/internal/cooperationfeishu"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
)

func (s *AdminServer) SetCooperationFeishu(sync *cooperationfeishu.Service) {
	s.cooperationFeishu = sync
}

func (s *AdminServer) GetCooperationFeishu(ctx echo.Context) error {
	id, err := cooperationResourceID(ctx)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	if s.cooperationFeishu == nil {
		return response.BadRequest(ctx, "合作申请飞书同步尚未配置")
	}
	target, err := s.cooperationFeishu.Get(ctx.Request().Context(), currentAdminID(ctx), id)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return response.Success(ctx, target)
}

func (s *AdminServer) StartCooperationFeishu(ctx echo.Context) error {
	id, err := cooperationResourceID(ctx)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	if s.cooperationFeishu == nil {
		return response.BadRequest(ctx, "合作申请飞书同步尚未配置")
	}
	target, err := s.cooperationFeishu.Start(ctx.Request().Context(), currentAdminID(ctx), id)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return response.Success(ctx, target)
}
