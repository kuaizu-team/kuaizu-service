package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	adminvo "github.com/kuaizu-team/kuaizu-service/internal/admin/vo"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
)

type adminEventRequest struct {
	Name                 string  `json:"name"`
	IsRanking            *bool   `json:"isRanking"`
	RegistrationDeadline *string `json:"registrationDeadline"`
	ArticleURL           *string `json:"articleUrl"`
	DisplayOrder         *int    `json:"displayOrder"`
	CreatedAt            *string `json:"createdAt"`
	Level                *string `json:"level"`
	Summary              *string `json:"summary"`
	OrganizerName        *string `json:"organizerName"`
	Description          *string `json:"description"`
	ResourceURL          *string `json:"resourceUrl"`
	QQGroup              *string `json:"qqGroup"`
	AllowCrossSchool     *bool   `json:"allowCrossSchool"`
	AllowCrossMajor      *bool   `json:"allowCrossMajor"`
	CrossSchoolMajorRule *string `json:"crossSchoolMajorRule"`
	ParticipationMode    *string `json:"participationMode"`
	TeamMinMembers       *int    `json:"teamMinMembers"`
	TeamMaxMembers       *int    `json:"teamMaxMembers"`
	SchoolID             *int    `json:"schoolId"`
	ManagerAccount       *string `json:"managerAccount"`
	ManagerPassword      *string `json:"managerPassword"`
	ManagerPhone         *string `json:"managerPhone"`
}

type adminEventMergeRequest struct {
	TargetEventID int `json:"targetEventId"`
}

type adminEventManagerVO struct {
	ID       int     `json:"id"`
	Nickname *string `json:"nickname"`
	Phone    *string `json:"phone"`
	Account  *string `json:"account,omitempty"`
}

type adminEventPermissionsVO struct {
	CanEdit               bool `json:"canEdit"`
	CanEditDeadline       bool `json:"canEditDeadline"`
	CanMerge              bool `json:"canMerge"`
	CanDelete             bool `json:"canDelete"`
	CanCreateEventManager bool `json:"canCreateEventManager"`
	CanEditEventManager   bool `json:"canEditEventManager"`
}

type adminEventDetailVO struct {
	*adminvo.AdminEventVO
	Manager     *adminEventManagerVO       `json:"manager"`
	Permissions adminEventPermissionsVO    `json:"permissions"`
	Timeline    []models.EventTimelineNode `json:"timeline"`
}

type adminEventTimelineItemRequest struct {
	Title       string  `json:"title"`
	NodeTime    string  `json:"nodeTime"`
	Description *string `json:"description"`
	SortOrder   int     `json:"sortOrder"`
}

type adminEventTimelineRequest struct {
	Items []adminEventTimelineItemRequest `json:"items"`
}

type eventManagerExecer interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

func canManageEvents(role int) bool {
	return role == models.AdminRoleSuperAdmin ||
		role == models.AdminRoleSchoolSuperAdmin ||
		role == models.AdminRoleSchoolAdmin
}

func canMergeEvents(role int) bool {
	return role == models.AdminRoleSuperAdmin
}

func requireEventManagementRole(ctx echo.Context) error {
	if !canManageEvents(adminRole(ctx)) {
		return response.Forbidden(ctx, "event management requires a super admin role")
	}
	return nil
}

func (s *AdminServer) ListEvents(ctx echo.Context) error {
	if err := requireEventManagementRole(ctx); err != nil {
		return err
	}
	page, _ := strconv.Atoi(ctx.QueryParam("page"))
	size, _ := strconv.Atoi(ctx.QueryParam("size"))
	keyword := strings.TrimSpace(ctx.QueryParam("keyword"))
	var keywordPtr *string
	if keyword != "" {
		keywordPtr = &keyword
	}
	sortBy := strings.TrimSpace(ctx.QueryParam("sortBy"))
	switch sortBy {
	case "", "updatedAt", "id", "registrationDeadline", "displayOrder", "projectCount":
	default:
		return response.BadRequest(ctx, "invalid sortBy")
	}
	if sortBy == "" {
		sortBy = "updatedAt"
	}
	order := strings.ToLower(strings.TrimSpace(ctx.QueryParam("order")))
	if order == "" {
		order = "desc"
	}
	if order != "asc" && order != "desc" {
		return response.BadRequest(ctx, "invalid order")
	}
	listParams := repository.EventListParams{
		Page: page, Size: size, Keyword: keywordPtr, SortBy: sortBy, Order: order,
	}
	if adminRole(ctx) == models.AdminRoleSchoolSuperAdmin || adminRole(ctx) == models.AdminRoleSchoolAdmin {
		schoolIDs, err := s.adminSchoolIDs(ctx)
		if err != nil {
			return response.InternalError(ctx, "查询学校权限失败")
		}
		if schoolIDs == nil {
			schoolIDs = []int{}
		}
		listParams.SchoolIDs = schoolIDs
		listParams.ProjectSchoolIDs = schoolIDs
	}
	result, err := s.svc.Event.ListEvents(ctx.Request().Context(), listParams)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	list := make([]adminvo.AdminEventVO, len(result.List))
	for i := range result.List {
		item := adminvo.NewAdminEventVO(&result.List[i])
		if adminRole(ctx) != models.AdminRoleSuperAdmin {
			item.ManagerUsername = nil
			item.ManagerNickname = nil
		}
		list[i] = *item
	}
	return response.Success(ctx, map[string]interface{}{
		"list": list, "total": result.Total, "page": result.Page, "size": result.Size,
	})
}

func (s *AdminServer) GetEvent(ctx echo.Context) error {
	if err := requireEventManagementRole(ctx); err != nil {
		return err
	}
	id, err := parseIDParam(ctx, "id", "event")
	if err != nil {
		return err
	}
	var projectSchoolIDs []int
	if adminRole(ctx) == models.AdminRoleSchoolSuperAdmin || adminRole(ctx) == models.AdminRoleSchoolAdmin {
		projectSchoolIDs, err = s.adminSchoolIDs(ctx)
		if err != nil {
			return response.InternalError(ctx, "查询学校权限失败")
		}
		if projectSchoolIDs == nil {
			projectSchoolIDs = []int{}
		}
	}
	event, err := s.repo.Event.GetByIDWithProjectSchoolIDs(ctx.Request().Context(), id, projectSchoolIDs)
	if err != nil {
		return response.InternalError(ctx, "查询赛事失败")
	}
	if event == nil {
		return response.NotFound(ctx, "赛事不存在")
	}
	if !s.canViewEventInScope(ctx, event) {
		return response.Forbidden(ctx, "无权查看该赛事")
	}
	return response.Success(ctx, s.buildAdminEventDetailVO(ctx, event))
}

func (s *AdminServer) ReplaceEventTimeline(ctx echo.Context) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}
	id, err := parseIDParam(ctx, "id", "event")
	if err != nil {
		return err
	}
	event, err := s.repo.Event.GetByID(ctx.Request().Context(), id)
	if err != nil || event == nil {
		return response.NotFound(ctx, "event not found")
	}
	var req adminEventTimelineRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	items := make([]models.EventTimelineNode, 0, len(req.Items))
	for index, raw := range req.Items {
		title := strings.TrimSpace(raw.Title)
		if title == "" || len([]rune(title)) > 120 {
			return response.BadRequest(ctx, "时间节点标题不能为空且不能超过 120 字")
		}
		nodeTime, err := time.Parse(time.RFC3339, strings.TrimSpace(raw.NodeTime))
		if err != nil {
			return response.BadRequest(ctx, "时间节点时间格式无效")
		}
		var description *string
		if raw.Description != nil {
			value := strings.TrimSpace(*raw.Description)
			if len([]rune(value)) > 500 {
				return response.BadRequest(ctx, "时间节点描述不能超过 500 字")
			}
			if value != "" {
				description = &value
			}
		}
		sortOrder := raw.SortOrder
		if sortOrder == 0 {
			sortOrder = index
		}
		items = append(items, models.EventTimelineNode{EventID: id, Title: title, NodeTime: nodeTime, Description: description, SortOrder: sortOrder})
	}
	if err := s.repo.Event.ReplaceTimelineNodes(ctx.Request().Context(), id, items); err != nil {
		return response.InternalError(ctx, "保存赛事时间线失败")
	}
	saved, err := s.repo.Event.ListTimelineNodes(ctx.Request().Context(), id)
	if err != nil {
		return response.InternalError(ctx, "读取赛事时间线失败")
	}
	return response.Success(ctx, saved)
}

func (s *AdminServer) CreateEvent(ctx echo.Context) error {
	if err := requireEventManagementRole(ctx); err != nil {
		return err
	}
	var req adminEventRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	event, err := s.buildAdminEventModelForRequest(ctx, req)
	if err != nil {
		return response.BadRequest(ctx, err.Error())
	}
	creatorID := currentAdminID(ctx)
	event.CreatorID = &creatorID

	requestCtx := ctx.Request().Context()
	tx, err := s.repo.DB().BeginTxx(requestCtx, nil)
	if err != nil {
		return response.InternalError(ctx, "创建赛事失败")
	}
	defer tx.Rollback()
	if err := s.svc.Event.CreateEventTx(requestCtx, tx, event); err != nil {
		return mapServiceError(ctx, err)
	}
	if err := s.upsertEventManager(ctx, tx, event, req); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return response.InternalError(ctx, "创建赛事失败")
	}

	created, err := s.repo.Event.GetByID(requestCtx, event.ID)
	if err != nil || created == nil {
		return response.InternalError(ctx, "获取赛事信息失败")
	}
	return response.Success(ctx, adminvo.NewAdminEventVO(created))
}

func (s *AdminServer) UpdateEvent(ctx echo.Context) error {
	if err := requireEventManagementRole(ctx); err != nil {
		return err
	}
	id, err := parseIDParam(ctx, "id", "event")
	if err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(ctx.Request().Body).Decode(&raw); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	payload, _ := json.Marshal(raw)
	var req adminEventRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	requestCtx := ctx.Request().Context()
	existing, err := s.repo.Event.GetByID(requestCtx, id)
	if err != nil || existing == nil {
		return response.NotFound(ctx, "event not found")
	}
	if !s.canViewEventInScope(ctx, existing) {
		return response.Forbidden(ctx, "无权编辑该赛事")
	}
	if adminRole(ctx) != models.AdminRoleSuperAdmin {
		return s.updateScopedEvent(ctx, existing, req, raw)
	}
	event, err := s.buildAdminEventModelForRequest(ctx, req)
	if err != nil {
		return response.BadRequest(ctx, err.Error())
	}
	event.ID = id
	event.AdminID = existing.AdminID
	if req.ParticipationMode == nil {
		event.ParticipationMode = existing.ParticipationMode
		event.TeamMinMembers = existing.TeamMinMembers
		event.TeamMaxMembers = existing.TeamMaxMembers
	}

	tx, err := s.repo.DB().BeginTxx(requestCtx, nil)
	if err != nil {
		return response.InternalError(ctx, "更新赛事失败")
	}
	defer tx.Rollback()
	if err := s.svc.Event.UpdateEventTx(requestCtx, tx, event); err != nil {
		return mapServiceError(ctx, err)
	}
	if err := s.upsertEventManager(ctx, tx, event, req); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return response.InternalError(ctx, "更新赛事失败")
	}

	updated, err := s.repo.Event.GetByID(requestCtx, id)
	if err != nil || updated == nil {
		return response.InternalError(ctx, "获取赛事信息失败")
	}
	return response.Success(ctx, adminvo.NewAdminEventVO(updated))
}

func (s *AdminServer) updateScopedEvent(ctx echo.Context, event *models.Event, req adminEventRequest, raw map[string]json.RawMessage) error {
	managerAllowed := event.AdminID == nil && s.canCreateEventManager(ctx, event) ||
		event.AdminID != nil && s.canEditEventManager(ctx, event)
	for field := range raw {
		switch field {
		case "registrationDeadline":
		case "managerAccount", "managerPassword", "managerPhone":
			if !managerAllowed {
				return response.Forbidden(ctx, "无权修改该赛事管理员")
			}
		default:
			return response.Forbidden(ctx, "校区角色只能修改报名截止时间和获授权的赛事管理员信息")
		}
	}
	if len(raw) == 0 {
		return response.BadRequest(ctx, "request body cannot be empty")
	}

	var deadline *time.Time
	if _, ok := raw["registrationDeadline"]; ok && req.RegistrationDeadline != nil &&
		strings.TrimSpace(*req.RegistrationDeadline) != "" {
		parsed, err := models.ParseEventDate(strings.TrimSpace(*req.RegistrationDeadline))
		if err != nil {
			return response.BadRequest(ctx, "报名截止时间格式无效")
		}
		deadline = &parsed
	}

	requestCtx := ctx.Request().Context()
	tx, err := s.repo.DB().BeginTxx(requestCtx, nil)
	if err != nil {
		return response.InternalError(ctx, "更新赛事失败")
	}
	defer tx.Rollback()
	if _, ok := raw["registrationDeadline"]; ok {
		if _, err := tx.ExecContext(requestCtx, `UPDATE event SET registration_deadline=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, deadline, event.ID); err != nil {
			return response.InternalError(ctx, "更新报名截止时间失败")
		}
	}
	if _, account := raw["managerAccount"]; account {
		if err := s.upsertEventManager(ctx, tx, event, req); err != nil {
			return err
		}
	} else if _, password := raw["managerPassword"]; password {
		if err := s.upsertEventManager(ctx, tx, event, req); err != nil {
			return err
		}
	} else if _, phone := raw["managerPhone"]; phone {
		if err := s.upsertEventManager(ctx, tx, event, req); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return response.InternalError(ctx, "更新赛事失败")
	}
	updated, err := s.repo.Event.GetByID(requestCtx, event.ID)
	if err != nil || updated == nil {
		return response.InternalError(ctx, "获取赛事信息失败")
	}
	return response.Success(ctx, adminvo.NewAdminEventVO(updated))
}

func (s *AdminServer) canViewEventInScope(ctx echo.Context, event *models.Event) bool {
	if event == nil {
		return false
	}
	role := adminRole(ctx)
	if role == models.AdminRoleSuperAdmin {
		return true
	}
	if role != models.AdminRoleSchoolSuperAdmin && role != models.AdminRoleSchoolAdmin {
		return false
	}
	if event.Level == nil || *event.Level != "school" {
		return true
	}
	allowed, err := s.canAccessSchool(ctx, event.SchoolID)
	return err == nil && allowed
}

func (s *AdminServer) isOwnSchoolEvent(ctx echo.Context, event *models.Event) bool {
	if event == nil || event.Level == nil || *event.Level != "school" {
		return false
	}
	allowed, err := s.canAccessSchool(ctx, event.SchoolID)
	return err == nil && allowed
}

func (s *AdminServer) canViewEventManagerCredentials(ctx echo.Context, event *models.Event) bool {
	switch adminRole(ctx) {
	case models.AdminRoleSuperAdmin:
		return true
	case models.AdminRoleSchoolSuperAdmin:
		return s.isOwnSchoolEvent(ctx, event) && event.CreatorID != nil && *event.CreatorID == currentAdminID(ctx)
	case models.AdminRoleSchoolAdmin:
		return s.isOwnSchoolEvent(ctx, event)
	default:
		return false
	}
}

func (s *AdminServer) canCreateEventManager(ctx echo.Context, event *models.Event) bool {
	if adminRole(ctx) == models.AdminRoleSuperAdmin {
		return true
	}
	if adminRole(ctx) == models.AdminRoleSchoolAdmin {
		return s.isOwnSchoolEvent(ctx, event)
	}
	return adminRole(ctx) == models.AdminRoleSchoolSuperAdmin &&
		s.isOwnSchoolEvent(ctx, event) && event.CreatorID != nil && *event.CreatorID == currentAdminID(ctx)
}

func (s *AdminServer) canEditEventManager(ctx echo.Context, event *models.Event) bool {
	return adminRole(ctx) == models.AdminRoleSuperAdmin ||
		adminRole(ctx) == models.AdminRoleSchoolSuperAdmin &&
			s.isOwnSchoolEvent(ctx, event) && event.CreatorID != nil && *event.CreatorID == currentAdminID(ctx)
}

func (s *AdminServer) buildAdminEventDetailVO(ctx echo.Context, event *models.Event) *adminEventDetailVO {
	detail := &adminEventDetailVO{
		AdminEventVO: adminvo.NewAdminEventVO(event),
		Permissions: adminEventPermissionsVO{
			CanEdit:               adminRole(ctx) == models.AdminRoleSuperAdmin,
			CanEditDeadline:       adminRole(ctx) != models.AdminRoleEventManager,
			CanMerge:              canMergeEvents(adminRole(ctx)),
			CanDelete:             adminRole(ctx) == models.AdminRoleSuperAdmin,
			CanCreateEventManager: event.AdminID == nil && s.canCreateEventManager(ctx, event),
			CanEditEventManager:   event.AdminID != nil && s.canEditEventManager(ctx, event),
		},
	}
	detail.Timeline, _ = s.repo.Event.ListTimelineNodes(ctx.Request().Context(), event.ID)
	detail.ManagerUsername = nil
	detail.ManagerNickname = nil
	if event.AdminID == nil {
		return detail
	}
	manager, err := s.repo.AdminUser.GetByID(ctx.Request().Context(), *event.AdminID)
	if err != nil || manager == nil || manager.Role != models.AdminRoleEventManager {
		return detail
	}
	managerSchoolMatches := schoolIDsMatch(event.SchoolID, manager.SchoolID)
	if adminRole(ctx) != models.AdminRoleSuperAdmin && event.Level != nil && *event.Level == "school" &&
		!managerSchoolMatches {
		return detail
	}
	detail.Manager = &adminEventManagerVO{ID: manager.ID, Nickname: manager.Nickname, Phone: manager.Phone}
	if s.canViewEventManagerCredentials(ctx, event) &&
		(adminRole(ctx) == models.AdminRoleSuperAdmin || managerSchoolMatches) {
		account := manager.Username
		detail.Manager.Account = &account
	}
	return detail
}

func (s *AdminServer) buildAdminEventModelForRequest(ctx echo.Context, req adminEventRequest) (*models.Event, error) {
	role := adminRole(ctx)
	if role != models.AdminRoleSchoolSuperAdmin && role != models.AdminRoleSchoolAdmin {
		return buildAdminEventModel(req, adminRole(ctx), adminSchoolID(ctx))
	}
	if req.Level == nil || strings.TrimSpace(*req.Level) != "school" {
		return nil, errors.New("school administrators can only manage school-level events")
	}
	if role == models.AdminRoleSchoolAdmin {
		schoolID := adminSchoolID(ctx)
		if schoolID == nil {
			return nil, errors.New("current admin is not associated with a school")
		}
		if req.SchoolID != nil && *req.SchoolID != *schoolID {
			return nil, errors.New("schoolId is outside the current admin scope")
		}
		return buildAdminEventModel(req, role, schoolID)
	}
	if req.SchoolID == nil {
		return nil, errors.New("schoolId is required for school-level events")
	}
	allowed, err := s.canAccessSchool(ctx, req.SchoolID)
	if err != nil || !allowed {
		return nil, errors.New("schoolId is outside the current admin scope")
	}
	return buildAdminEventModel(req, adminRole(ctx), req.SchoolID)
}

func (s *AdminServer) upsertEventManager(ctx echo.Context, exec eventManagerExecer, event *models.Event, req adminEventRequest) error {
	account := ""
	password := ""
	phone := ""
	if req.ManagerAccount != nil {
		account = strings.TrimSpace(*req.ManagerAccount)
	}
	if req.ManagerPassword != nil {
		password = strings.TrimSpace(*req.ManagerPassword)
	}
	if req.ManagerPhone != nil {
		phone = strings.TrimSpace(*req.ManagerPhone)
	}
	if event.AdminID == nil && account == "" && password == "" && phone == "" {
		return nil
	}
	managerAllowed := event.AdminID == nil && s.canCreateEventManager(ctx, event) ||
		event.AdminID != nil && s.canEditEventManager(ctx, event)
	if !managerAllowed {
		return response.Forbidden(ctx, "无权管理该赛事的赛事管理员")
	}
	if event.AdminID == nil && (account == "" || password == "" || phone == "") {
		return response.BadRequest(ctx, "新建赛事管理员时账号、密码和电话均为必填")
	}
	if phone != "" && !adminPhonePattern.MatchString(phone) {
		return response.BadRequest(ctx, "请输入正确的手机号")
	}
	if password != "" && len(password) < 6 {
		return response.BadRequest(ctx, "密码至少 6 位")
	}
	nickname := event.Name + "管理员"
	if event.AdminID == nil {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return response.InternalError(ctx, "密码加密失败")
		}
		result, err := exec.ExecContext(ctx.Request().Context(), `INSERT INTO admin_user(username,password_hash,nickname,phone,role,school_id,status) VALUES(?,?,?,?,?,?,1)`, account, string(hash), nickname, phone, models.AdminRoleEventManager, event.SchoolID)
		if err != nil {
			return response.BadRequest(ctx, "赛事管理员账号已存在")
		}
		id, err := result.LastInsertId()
		if err != nil {
			return response.InternalError(ctx, "读取赛事管理员信息失败")
		}
		linkResult, err := exec.ExecContext(ctx.Request().Context(), `UPDATE event SET admin_id=? WHERE id=? AND admin_id IS NULL`, id, event.ID)
		if err != nil {
			return response.InternalError(ctx, "关联赛事管理员失败")
		}
		if rows, _ := linkResult.RowsAffected(); rows != 1 {
			return response.InternalError(ctx, "关联赛事管理员失败")
		}
		return nil
	}
	sets := []string{"nickname=?", "school_id=?"}
	args := []interface{}{nickname, event.SchoolID}
	if account != "" {
		sets = append(sets, "username=?")
		args = append(args, account)
	}
	if phone != "" {
		sets = append(sets, "phone=?")
		args = append(args, phone)
	}
	if password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return response.InternalError(ctx, "密码加密失败")
		}
		sets = append(sets, "password_hash=?")
		args = append(args, string(hash))
	}
	args = append(args, *event.AdminID)
	managerScope := ""
	if adminRole(ctx) != models.AdminRoleSuperAdmin {
		managerScope = " AND school_id=?"
		args = append(args, event.SchoolID)
	}
	if _, err := exec.ExecContext(ctx.Request().Context(), `UPDATE admin_user SET `+strings.Join(sets, ",")+`,updated_at=CURRENT_TIMESTAMP WHERE id=? AND role=4`+managerScope, args...); err != nil {
		return response.BadRequest(ctx, "赛事管理员账号更新失败")
	}
	return nil
}

func (s *AdminServer) DeleteEvent(ctx echo.Context) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}
	id, err := parseIDParam(ctx, "id", "event")
	if err != nil {
		return err
	}
	if err := s.svc.Event.DeleteEvent(ctx.Request().Context(), id); err != nil {
		return mapServiceError(ctx, err)
	}
	return response.SuccessMessage(ctx, "operation succeeded")
}

func (s *AdminServer) MergeEvent(ctx echo.Context) error {
	if !canMergeEvents(adminRole(ctx)) {
		return response.Forbidden(ctx, "event merge requires a super admin role")
	}
	id, err := parseIDParam(ctx, "id", "event")
	if err != nil {
		return err
	}
	var req adminEventMergeRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	source, err := s.repo.Event.GetByID(ctx.Request().Context(), id)
	if err != nil || source == nil {
		return response.NotFound(ctx, "event not found")
	}
	target, err := s.repo.Event.GetByID(ctx.Request().Context(), req.TargetEventID)
	if err != nil || target == nil {
		return response.NotFound(ctx, "target event not found")
	}
	levelRank := map[string]int{"school": 1, "regional": 2, "national": 3}
	if source.Level == nil || target.Level == nil || levelRank[*target.Level] <= levelRank[*source.Level] {
		return response.BadRequest(ctx, "events can only be merged upward: school to regional to national")
	}
	merged, err := s.svc.Event.MergeEvent(ctx.Request().Context(), id, req.TargetEventID)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return response.Success(ctx, adminvo.NewAdminEventVO(merged))
}

func buildAdminEventModel(req adminEventRequest, role int, adminSchoolID *int) (*models.Event, error) {
	event := &models.Event{Name: strings.TrimSpace(req.Name), AllowCrossSchool: 1, AllowCrossMajor: 1}
	if req.Level != nil && strings.TrimSpace(*req.Level) != "" {
		level := strings.TrimSpace(*req.Level)
		if level != "national" && level != "regional" && level != "school" {
			return nil, errors.New("level must be national, regional, or school")
		}
		event.Level = &level
		if level == "school" {
			if role == models.AdminRoleSuperAdmin {
				if req.SchoolID == nil || *req.SchoolID <= 0 {
					return nil, errors.New("schoolId is required for school-level events")
				}
				event.SchoolID = req.SchoolID
			} else {
				if adminSchoolID == nil {
					return nil, errors.New("current admin is not associated with a school")
				}
				event.SchoolID = adminSchoolID
			}
		}
	}
	if req.Summary != nil {
		value := strings.TrimSpace(*req.Summary)
		if value != "" {
			event.Summary = &value
		}
	}
	optionalFields := []struct {
		input  *string
		target **string
	}{
		{req.OrganizerName, &event.OrganizerName},
		{req.Description, &event.Description},
		{req.ResourceURL, &event.ResourceURL},
		{req.QQGroup, &event.QQGroup},
	}
	for _, field := range optionalFields {
		if field.input == nil {
			continue
		}
		value := strings.TrimSpace(*field.input)
		if value != "" {
			*field.target = &value
		}
	}
	if req.AllowCrossSchool != nil && !*req.AllowCrossSchool {
		event.AllowCrossSchool = 0
	}
	if req.AllowCrossMajor != nil && !*req.AllowCrossMajor {
		event.AllowCrossMajor = 0
	}
	if req.CrossSchoolMajorRule != nil {
		value := strings.TrimSpace(*req.CrossSchoolMajorRule)
		event.CrossSchoolMajorRule = &value
	}
	if req.ParticipationMode != nil {
		value := strings.TrimSpace(*req.ParticipationMode)
		event.ParticipationMode = &value
	}
	event.TeamMinMembers = req.TeamMinMembers
	event.TeamMaxMembers = req.TeamMaxMembers
	if req.IsRanking != nil && *req.IsRanking {
		event.IsRanking = 1
	}
	if req.DisplayOrder != nil {
		event.DisplayOrder = *req.DisplayOrder
	}
	if req.ArticleURL != nil {
		value := strings.TrimSpace(*req.ArticleURL)
		if value != "" {
			event.ArticleURL = &value
		}
	}
	if req.RegistrationDeadline != nil && strings.TrimSpace(*req.RegistrationDeadline) != "" {
		t, err := models.ParseEventDate(strings.TrimSpace(*req.RegistrationDeadline))
		if err != nil {
			return nil, err
		}
		event.RegistrationDeadline = &t
	}
	if req.CreatedAt != nil && strings.TrimSpace(*req.CreatedAt) != "" {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(*req.CreatedAt))
		if err != nil {
			return nil, err
		}
		event.CreatedAt = t
	}
	return event, nil
}
