package handler

import (
	"database/sql"
	"errors"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/labstack/echo/v4"
	"strconv"
	"time"
)

// RegisterScheduleRoutes uses the existing authenticated API group.
func (s *Server) RegisterScheduleRoutes(g *echo.Group) {
	g.GET("/users/me/schedules", s.listSchedules)
	g.POST("/users/me/schedules", s.saveSchedule)
	g.PUT("/users/me/schedules/:id", s.saveSchedule)
	g.DELETE("/users/me/schedules/:id", s.deleteSchedule)
}
func (s *Server) scheduleRepo() *repository.ScheduleRepository {
	return repository.NewScheduleRepository(s.repo.DB())
}
func (s *Server) listSchedules(c echo.Context) error {
	from, to := c.QueryParam("from"), c.QueryParam("to")
	a, e1 := time.Parse("2006-01-02", from)
	b, e2 := time.Parse("2006-01-02", to)
	if e1 != nil || e2 != nil || b.Before(a) || b.Sub(a) > 93*24*time.Hour {
		return BadRequest(c, "请选择不超过 93 天的有效日期范围")
	}
	items, err := s.scheduleRepo().List(c.Request().Context(), GetUserID(c), from, to)
	if err != nil {
		c.Logger().Error("list schedules failed")
		return InternalError(c, "日程暂不可用，请稍后重试")
	}
	if len(items) > 2000 {
		return BadRequest(c, "日程数量较多，请缩小日期范围")
	}
	return Success(c, items)
}
func (s *Server) saveSchedule(c echo.Context) error {
	var item models.Schedule
	if err := c.Bind(&item); err != nil {
		return BadRequest(c, "日程格式错误")
	}
	if err := item.Validate(); err != nil {
		return BadRequest(c, err.Error())
	}
	item.ID = 0
	if c.Request().Method == "PUT" {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			return BadRequest(c, "无效日程")
		}
		item.ID = id
	}
	var err error
	if item.ID == 0 {
		err = s.scheduleRepo().Create(c.Request().Context(), GetUserID(c), &item)
	} else {
		err = s.scheduleRepo().Update(c.Request().Context(), GetUserID(c), &item)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return NotFound(c, "日程不存在")
	}
	if err != nil {
		c.Logger().Error("save schedule failed")
		return InternalError(c, "日程保存失败")
	}
	return Success(c, item)
}
func (s *Server) deleteSchedule(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return BadRequest(c, "无效日程")
	}
	err = s.scheduleRepo().Delete(c.Request().Context(), GetUserID(c), id)
	if errors.Is(err, sql.ErrNoRows) {
		return NotFound(c, "日程不存在")
	}
	if err != nil {
		return InternalError(c, "日程删除失败")
	}
	return SuccessMessage(c, "已删除日程")
}
