package handler

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/labstack/echo/v4"
	"net/http"
	"strconv"
	"time"
)

// RegisterScheduleRoutes uses the existing authenticated API group.
func (s *Server) RegisterScheduleRoutes(g *echo.Group) {
	g.GET("/users/me/schedules", s.listSchedules)
	g.POST("/users/me/schedules", s.saveSchedule)
	g.POST("/users/me/schedules/import", s.importSchedules)
	g.PUT("/users/me/schedules/:id", s.saveSchedule)
	g.DELETE("/users/me/schedules/:id", s.deleteSchedule)
}

type scheduleImportRequest struct {
	Items []models.Schedule `json:"items"`
}

func (s *Server) importSchedules(c echo.Context) error {
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, 1024*1024)
	var request scheduleImportRequest
	if err := c.Bind(&request); err != nil {
		return BadRequest(c, "导入数据格式错误或超过 1MB")
	}
	if len(request.Items) == 0 || len(request.Items) > 500 {
		return BadRequest(c, "一次需导入 1—500 次课程")
	}
	for i := range request.Items {
		request.Items[i].ID = 0
		if err := request.Items[i].Validate(); err != nil {
			return BadRequest(c, fmt.Sprintf("第 %d 条课程：%s", i+1, err.Error()))
		}
	}
	created, skipped, err := s.scheduleRepo().Import(c.Request().Context(), GetUserID(c), request.Items)
	if err != nil {
		c.Logger().Error("schedule import transaction failed")
		return InternalError(c, "课表导入失败，请稍后重试")
	}
	return Success(c, map[string]int{"created": created, "skipped": skipped})
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
