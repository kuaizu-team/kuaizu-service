package handler

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
)

func (s *Server) GetMyPendingStatusNotification(ctx echo.Context) error {
	notification, err := s.repo.StatusNotification.GetPending(ctx.Request().Context(), GetUserID(ctx))
	if err != nil {
		return InternalError(ctx, "获取状态通知失败")
	}
	return Success(ctx, notification)
}

func (s *Server) MarkMyStatusNotificationDisplayed(ctx echo.Context) error {
	id, err := strconv.ParseInt(ctx.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return BadRequest(ctx, "通知ID无效")
	}
	if err := s.repo.StatusNotification.MarkDisplayed(ctx.Request().Context(), id, GetUserID(ctx)); err != nil {
		if err == sql.ErrNoRows {
			return ctx.JSON(http.StatusNotFound, map[string]interface{}{"code": 404, "message": "通知不存在"})
		}
		return InternalError(ctx, "更新状态通知失败")
	}
	return SuccessMessage(ctx, "success")
}
