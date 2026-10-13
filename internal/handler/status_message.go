package handler

import (
	"database/sql"
	"errors"
	"strconv"

	"github.com/labstack/echo/v4"
)

func messageError(ctx echo.Context, err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ctx.JSON(404, map[string]interface{}{"code": 404, "message": "消息不存在"})
	}
	return InternalError(ctx, "消息操作失败")
}

func (s *Server) ListMyStatusMessages(ctx echo.Context) error {
	page, size := 1, 20
	for name, target := range map[string]*int{"page": &page, "size": &size} {
		if raw := ctx.QueryParam(name); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 1 || (name == "page" && value > 1000000) {
				return BadRequest(ctx, "分页参数无效")
			}
			*target = value
		}
	}
	if size > 50 {
		size = 50
	}
	var read *bool
	if raw := ctx.QueryParam("read"); raw != "" {
		if raw != "true" && raw != "false" {
			return BadRequest(ctx, "已读筛选参数无效")
		}
		value := raw == "true"
		read = &value
	}
	list, total, err := s.repo.StatusNotification.ListMessages(ctx.Request().Context(), GetUserID(ctx), page, size, read)
	if err != nil {
		return messageError(ctx, err)
	}
	return Success(ctx, map[string]interface{}{"list": list, "total": total, "page": page, "size": size})
}

func (s *Server) GetMyStatusMessage(ctx echo.Context) error {
	id, err := strconv.ParseInt(ctx.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return BadRequest(ctx, "消息ID无效")
	}
	m, err := s.repo.StatusNotification.GetMessage(ctx.Request().Context(), GetUserID(ctx), id)
	if err != nil {
		return messageError(ctx, err)
	}
	return Success(ctx, m)
}

func (s *Server) GetMyLatestStatusMessage(ctx echo.Context) error {
	kind := ctx.QueryParam("type")
	switch kind {
	case "application-accepted", "application-rejected", "olive-accepted", "olive-rejected", "member-removed",
		"recruit-completed", "project-ended", "talent-published", "project-published", "application-discussing", "super-admin-invitation":
	default:
		return BadRequest(ctx, "消息类型无效")
	}
	projectID := 0
	if raw := ctx.QueryParam("projectId"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			return BadRequest(ctx, "项目ID无效")
		}
		projectID = value
	}
	m, err := s.repo.StatusNotification.LatestMessage(ctx.Request().Context(), GetUserID(ctx), kind, projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return Success(ctx, nil)
	}
	if err != nil {
		return messageError(ctx, err)
	}
	return Success(ctx, m)
}

func (s *Server) GetMyUnreadStatusMessageCount(ctx echo.Context) error {
	count, err := s.repo.StatusNotification.UnreadMessageCount(ctx.Request().Context(), GetUserID(ctx))
	if err != nil {
		return messageError(ctx, err)
	}
	return Success(ctx, map[string]interface{}{"count": count})
}

func (s *Server) MarkMyStatusMessageRead(ctx echo.Context) error {
	return s.markMyStatusMessage(ctx, false)
}
func (s *Server) MarkMyStatusMessageDisplayed(ctx echo.Context) error {
	return s.markMyStatusMessage(ctx, true)
}
func (s *Server) markMyStatusMessage(ctx echo.Context, displayed bool) error {
	id, err := strconv.ParseInt(ctx.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return BadRequest(ctx, "消息ID无效")
	}
	read := !displayed
	if displayed {
		var body struct {
			Read bool `json:"read"`
		}
		if err := ctx.Bind(&body); err != nil {
			return BadRequest(ctx, "请求参数无效")
		}
		read = body.Read
	}
	if err := s.repo.StatusNotification.MarkMessage(ctx.Request().Context(), GetUserID(ctx), id, displayed, read); err != nil {
		return messageError(ctx, err)
	}
	return SuccessMessage(ctx, "success")
}
