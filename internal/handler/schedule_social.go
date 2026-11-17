package handler

import (
	"errors"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/labstack/echo/v4"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func (s *Server) registerScheduleSocial(g *echo.Group) {
	base := "/users/me/schedule-social"
	g.GET(base+"/privacy", s.schedulePrivacy)
	g.PUT(base+"/privacy", s.schedulePrivacy)
	g.GET(base+"/shares", s.scheduleShares)
	g.POST(base+"/shares", s.scheduleShares)
	g.DELETE(base+"/shares/:id", s.scheduleRevoke)
	g.GET(base+"/view/:token", s.scheduleViewShare)
	g.POST(base+"/copy/:token", s.scheduleCopyShare)
	g.GET(base+"/teams", s.scheduleTeams)
	g.GET(base+"/teams/:id", s.scheduleTeam)
	g.POST(base+"/joint", s.scheduleJointCreate)
	g.GET(base+"/joint/:id", s.scheduleJointPeople)
	g.DELETE(base+"/joint/:id", s.scheduleJointLeave)
	g.GET(base+"/notices", s.scheduleNotices)
	g.PUT(base+"/notices", s.scheduleNotices)
}
func scheduleSocialError(c echo.Context, err error) error {
	if errors.Is(err, repository.ErrScheduleAccess) {
		return NotFound(c, "分享已失效或没有访问权限")
	}
	if errors.Is(err, repository.ErrScheduleRule) {
		return BadRequest(c, err.Error())
	}
	c.Logger().Error("schedule social operation failed")
	return InternalError(c, "日程操作失败，请稍后重试")
}
func scheduleSocialDate(date string) bool {
	d, err := time.Parse("2006-01-02", date)
	return err == nil && d.Format("2006-01-02") == date && date >= "2000-01-01" && date <= "2100-12-31"
}
func scheduleSocialID(c echo.Context) int64 {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	return id
}

var scheduleTokenPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func (s *Server) schedulePrivacy(c echo.Context) error {
	uid := GetUserID(c)
	if c.Request().Method == http.MethodPut {
		var p models.SchedulePrivacy
		if err := c.Bind(&p); err != nil {
			return BadRequest(c, "隐私设置无效")
		}
		if err := s.scheduleRepo().SavePrivacy(c.Request().Context(), uid, p); err != nil {
			return scheduleSocialError(c, err)
		}
	}
	p, err := s.scheduleRepo().Privacy(c.Request().Context(), uid)
	if err != nil {
		return scheduleSocialError(c, err)
	}
	return Success(c, p)
}
func (s *Server) scheduleShares(c echo.Context) error {
	uid := GetUserID(c)
	ctx := c.Request().Context()
	if c.Request().Method == http.MethodGet {
		items, err := s.scheduleRepo().Shares(ctx, uid)
		if err != nil {
			return scheduleSocialError(c, err)
		}
		return Success(c, items)
	}
	var body struct {
		Kind string `json:"kind"`
		Date string `json:"date"`
		ID   int64  `json:"id"`
	}
	if err := c.Bind(&body); err != nil {
		return BadRequest(c, "分享参数错误")
	}
	if body.Kind != "item" && body.Kind != "day" && body.Kind != "calendar" {
		return BadRequest(c, "无效分享范围")
	}
	if body.Kind == "day" && !scheduleSocialDate(body.Date) {
		return BadRequest(c, "分享日期无效")
	}
	if body.Kind == "item" && body.ID <= 0 {
		return BadRequest(c, "无效日程")
	}
	share, err := s.scheduleRepo().Share(ctx, uid, body.Kind, body.Date, body.ID)
	if err != nil {
		return scheduleSocialError(c, err)
	}
	return Success(c, share)
}
func (s *Server) scheduleRevoke(c echo.Context) error {
	if scheduleSocialID(c) <= 0 {
		return BadRequest(c, "分享无效")
	}
	if err := s.scheduleRepo().Revoke(c.Request().Context(), GetUserID(c), scheduleSocialID(c)); err != nil {
		return scheduleSocialError(c, err)
	}
	return SuccessMessage(c, "分享已撤销")
}
func (s *Server) scheduleViewShare(c echo.Context) error {
	token := c.Param("token")
	if !scheduleTokenPattern.MatchString(token) || !scheduleSocialDate(c.QueryParam("date")) {
		return BadRequest(c, "分享或日期无效")
	}
	share, items, err := s.scheduleRepo().ViewShare(c.Request().Context(), token, c.QueryParam("date"))
	if err != nil {
		return scheduleSocialError(c, err)
	}
	return Success(c, map[string]interface{}{"kind": share.Kind, "date": share.Date, "ownerId": share.OwnerID, "items": items})
}
func (s *Server) scheduleCopyShare(c echo.Context) error {
	var body struct {
		Allow bool `json:"allowConflicts"`
	}
	if err := c.Bind(&body); err != nil || !scheduleTokenPattern.MatchString(c.Param("token")) {
		return BadRequest(c, "分享参数错误")
	}
	result, err := s.scheduleRepo().CopyShare(c.Request().Context(), GetUserID(c), c.Param("token"), body.Allow)
	if err != nil {
		return scheduleSocialError(c, err)
	}
	return Success(c, result)
}
func (s *Server) scheduleTeams(c echo.Context) error {
	items, err := s.scheduleRepo().Teams(c.Request().Context(), GetUserID(c))
	if err != nil {
		return scheduleSocialError(c, err)
	}
	return Success(c, items)
}
func (s *Server) scheduleTeam(c echo.Context) error {
	pid := int(scheduleSocialID(c))
	if pid <= 0 {
		return BadRequest(c, "项目无效")
	}
	ctx := c.Request().Context()
	uid := GetUserID(c)
	people, err := s.scheduleRepo().TeamPeople(ctx, uid, pid)
	if err != nil {
		return scheduleSocialError(c, err)
	}
	if c.QueryParam("users") == "" {
		return Success(c, map[string]interface{}{"people": people})
	}
	parts := strings.Split(c.QueryParam("users"), ",")
	if len(parts) > 20 || !scheduleSocialDate(c.QueryParam("date")) {
		return BadRequest(c, "最多选择20人及有效日期")
	}
	ids := []int{}
	seen := map[int]bool{}
	for _, part := range parts {
		id, err := strconv.Atoi(part)
		if err != nil || id <= 0 || seen[id] {
			return BadRequest(c, "参与人无效")
		}
		seen[id] = true
		ids = append(ids, id)
	}
	busy, err := s.scheduleRepo().TeamBusy(ctx, uid, pid, ids, c.QueryParam("date"))
	if err != nil {
		return scheduleSocialError(c, err)
	}
	return Success(c, map[string]interface{}{"people": people, "busy": busy})
}
func (s *Server) scheduleJointCreate(c echo.Context) error {
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, 64*1024)
	var body struct {
		Item      models.Schedule `json:"item"`
		ProjectID int             `json:"projectId"`
		Token     string          `json:"token"`
		Key       string          `json:"requestKey"`
		IDs       []int           `json:"userIds"`
		Allow     bool            `json:"allowConflicts"`
	}
	if err := c.Bind(&body); err != nil {
		return BadRequest(c, "共同日程参数无效")
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9-]{16,64}$`).MatchString(body.Key) || len(body.IDs) > 20 {
		return BadRequest(c, "请求标识或参与人数无效")
	}
	if body.Token != "" {
		if body.ProjectID != 0 || !scheduleTokenPattern.MatchString(body.Token) {
			return BadRequest(c, "来源无效")
		}
	} else if body.ProjectID <= 0 {
		return BadRequest(c, "请选择项目团队或日历分享")
	}
	if err := body.Item.Validate(); err != nil {
		return BadRequest(c, err.Error())
	}
	if !body.Item.AllDay && body.Item.EndTime == nil {
		return BadRequest(c, "共同日程需填写结束时间")
	}
	result, err := s.scheduleRepo().CreateJoint(c.Request().Context(), GetUserID(c), body.ProjectID, body.Token, body.Key, body.IDs, body.Item, body.Allow)
	if err != nil {
		return scheduleSocialError(c, err)
	}
	return Success(c, result)
}
func (s *Server) scheduleJointPeople(c echo.Context) error {
	if scheduleSocialID(c) <= 0 {
		return BadRequest(c, "日程无效")
	}
	people, err := s.scheduleRepo().JointPeople(c.Request().Context(), GetUserID(c), scheduleSocialID(c))
	if err != nil {
		return scheduleSocialError(c, err)
	}
	return Success(c, people)
}
func (s *Server) scheduleJointLeave(c echo.Context) error {
	if scheduleSocialID(c) <= 0 {
		return BadRequest(c, "日程无效")
	}
	if err := s.scheduleRepo().LeaveJoint(c.Request().Context(), GetUserID(c), scheduleSocialID(c)); err != nil {
		return scheduleSocialError(c, err)
	}
	return SuccessMessage(c, "已退出或取消共同日程")
}
func (s *Server) scheduleNotices(c echo.Context) error {
	uid := GetUserID(c)
	ctx := c.Request().Context()
	if c.Request().Method == http.MethodPut {
		var body struct {
			Through int64 `json:"through"`
		}
		if err := c.Bind(&body); err != nil || body.Through <= 0 {
			return BadRequest(c, "通知无效")
		}
		if err := s.scheduleRepo().ReadNotices(ctx, uid, body.Through); err != nil {
			return scheduleSocialError(c, err)
		}
		return SuccessMessage(c, "已读")
	}
	items, err := s.scheduleRepo().Notices(ctx, uid)
	if err != nil {
		return scheduleSocialError(c, err)
	}
	return Success(c, items)
}
