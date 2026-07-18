package handler

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
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
	SchoolID             *int    `json:"schoolId"`
	ManagerAccount       *string `json:"managerAccount"`
	ManagerPassword      *string `json:"managerPassword"`
}

type adminEventMergeRequest struct {
	TargetEventID int `json:"targetEventId"`
}

func (s *AdminServer) ListEvents(ctx echo.Context) error {
	page, _ := strconv.Atoi(ctx.QueryParam("page"))
	size, _ := strconv.Atoi(ctx.QueryParam("size"))
	keyword := strings.TrimSpace(ctx.QueryParam("keyword"))
	var keywordPtr *string
	if keyword != "" {
		keywordPtr = &keyword
	}
	listParams := repository.EventListParams{
		Page: page, Size: size, Keyword: keywordPtr,
	}
	if adminRole(ctx) == models.AdminRoleSchoolSuperAdmin {
		schoolIDs, err := s.adminSchoolIDs(ctx)
		if err != nil {
			return response.InternalError(ctx, "查询学校权限失败")
		}
		listParams.SchoolIDs = schoolIDs
	}
	result, err := s.svc.Event.ListEvents(ctx.Request().Context(), listParams)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	list := make([]adminvo.AdminEventVO, len(result.List))
	for i := range result.List {
		list[i] = *adminvo.NewAdminEventVO(&result.List[i])
	}
	return response.Success(ctx, map[string]interface{}{
		"list": list, "total": result.Total, "page": result.Page, "size": result.Size,
	})
}

func (s *AdminServer) CreateEvent(ctx echo.Context) error {
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
	id, err := parseIDParam(ctx, "id", "event")
	if err != nil {
		return err
	}
	var req adminEventRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	requestCtx := ctx.Request().Context()
	existing, err := s.repo.Event.GetByID(requestCtx, id)
	if err != nil || existing == nil {
		return response.NotFound(ctx, "event not found")
	}
	if !s.canManageEventInScope(ctx, existing) {
		return response.Forbidden(ctx, "只能编辑本校学校层级赛事")
	}
	event, err := s.buildAdminEventModelForRequest(ctx, req)
	if err != nil {
		return response.BadRequest(ctx, err.Error())
	}
	event.ID = id
	event.AdminID = existing.AdminID

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
func (s *AdminServer) canManageEventInScope(ctx echo.Context, event *models.Event) bool {
	if adminRole(ctx) == models.AdminRoleSuperAdmin {
		return true
	}
	if adminRole(ctx) != models.AdminRoleSchoolSuperAdmin || event == nil || event.Level == nil || *event.Level != "school" {
		return false
	}
	allowed, err := s.canAccessSchool(ctx, event.SchoolID)
	return err == nil && allowed
}

func (s *AdminServer) buildAdminEventModelForRequest(ctx echo.Context, req adminEventRequest) (*models.Event, error) {
	if adminRole(ctx) != models.AdminRoleSchoolSuperAdmin {
		return buildAdminEventModel(req, adminRole(ctx), adminSchoolID(ctx))
	}
	if req.Level == nil || strings.TrimSpace(*req.Level) != "school" {
		return nil, errors.New("school super admins can only manage school-level events")
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

func (s *AdminServer) upsertEventManager(ctx echo.Context, exec sqlx.ExtContext, event *models.Event, req adminEventRequest) error {
	account := ""
	password := ""
	if req.ManagerAccount != nil {
		account = strings.TrimSpace(*req.ManagerAccount)
	}
	if req.ManagerPassword != nil {
		password = strings.TrimSpace(*req.ManagerPassword)
	}
	if account == "" && password == "" {
		return nil
	}
	if !s.canManageEventInScope(ctx, event) {
		return response.Forbidden(ctx, "无权管理该赛事的赛事管理员")
	}
	if event.AdminID == nil && (account == "" || password == "") {
		return response.BadRequest(ctx, "新建赛事管理员时账号和密码均为必填")
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
		encrypted, err := encryptAdminCredential(password)
		if err != nil {
			return response.InternalError(ctx, "赛事管理员密码安全存储失败")
		}
		result, err := exec.ExecContext(ctx.Request().Context(), `INSERT INTO admin_user(username,password_hash,password_encrypted,nickname,role,school_id,status) VALUES(?,?,?,?,?,?,1)`, account, string(hash), encrypted, nickname, models.AdminRoleEventManager, event.SchoolID)
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
	sets := []string{"nickname=?"}
	args := []interface{}{nickname}
	if account != "" {
		sets = append(sets, "username=?")
		args = append(args, account)
	}
	if password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return response.InternalError(ctx, "密码加密失败")
		}
		encrypted, err := encryptAdminCredential(password)
		if err != nil {
			return response.InternalError(ctx, "赛事管理员密码安全存储失败")
		}
		sets = append(sets, "password_hash=?", "password_encrypted=?")
		args = append(args, string(hash), encrypted)
	}
	args = append(args, *event.AdminID)
	if _, err := exec.ExecContext(ctx.Request().Context(), `UPDATE admin_user SET `+strings.Join(sets, ",")+`,updated_at=CURRENT_TIMESTAMP WHERE id=? AND role=4`, args...); err != nil {
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
	if adminRole(ctx) == models.AdminRoleSchoolSuperAdmin && !s.canManageEventInScope(ctx, source) {
		return response.Forbidden(ctx, "只能合并自己负责学校的赛事")
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
	event := &models.Event{Name: strings.TrimSpace(req.Name)}
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
