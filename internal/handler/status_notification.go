package handler

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/labstack/echo/v4"
)

func (s *Server) GetMyPendingStatusNotification(ctx echo.Context) error {
	includeCooperation, err := cooperationClientCapability(ctx)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	userID := GetUserID(ctx)
	if s.svc != nil && s.svc.Invitation != nil && s.repo.StatusNotification != nil {
		pendingInvitation, err := s.svc.Invitation.GetPendingInvitation(ctx.Request().Context(), userID)
		if err != nil {
			return mapServiceError(ctx, err)
		}
		if pendingInvitation != nil {
			if err := s.repo.StatusNotification.MarkAllPendingDisplayed(ctx.Request().Context(), userID); err != nil {
				return InternalError(ctx, "更新状态通知失败")
			}
			return Success(ctx, nil)
		}
	}

	var notification *models.StatusNotification
	if includeCooperation {
		capable, ok := s.repo.StatusNotification.(interface {
			GetPendingIncludingCooperation(context.Context, int) (*models.StatusNotification, error)
		})
		if !ok {
			return InternalError(ctx, "合作通知暂不可用")
		}
		notification, err = capable.GetPendingIncludingCooperation(ctx.Request().Context(), userID)
	} else {
		notification, err = s.repo.StatusNotification.GetPending(ctx.Request().Context(), userID)
	}
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
