package handler

import (
	"database/sql"
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
	result, err := s.svc.Event.ListEvents(ctx.Request().Context(), repository.EventListParams{
		Page: page, Size: size, Keyword: keywordPtr,
	})
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
	event, err := buildAdminEventModel(req, adminRole(ctx), adminSchoolID(ctx))
	if err != nil {
		return response.BadRequest(ctx, err.Error())
	}
	creatorID := currentAdminID(ctx)
	event.CreatorID = &creatorID
	created, err := s.svc.Event.CreateEvent(ctx.Request().Context(), event)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	if err := s.upsertEventManager(ctx, created, req); err != nil {
		_ = s.svc.Event.DeleteEvent(ctx.Request().Context(), created.ID)
		return err
	}
	created, _ = s.repo.Event.GetByID(ctx.Request().Context(), created.ID)
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
	existing, err := s.repo.Event.GetByID(ctx.Request().Context(), id)
	if err != nil || existing == nil {
		return response.NotFound(ctx, "event not found")
	}
	if !canManageEventManager(adminRole(ctx), adminSchoolID(ctx), existing) {
		return response.Forbidden(ctx, "只能编辑本校学校层级赛事")
	}
	event, err := buildAdminEventModel(req, adminRole(ctx), adminSchoolID(ctx))
	if err != nil {
		return response.BadRequest(ctx, err.Error())
	}
	event.ID = id
	updated, err := s.svc.Event.UpdateEvent(ctx.Request().Context(), event)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return response.NotFound(ctx, "event not found")
		}
		return mapServiceError(ctx, err)
	}
	updated.AdminID = existing.AdminID
	if err := s.upsertEventManager(ctx, updated, req); err != nil {
		return err
	}
	updated, _ = s.repo.Event.GetByID(ctx.Request().Context(), id)
	return response.Success(ctx, adminvo.NewAdminEventVO(updated))
}

func canManageEventManager(role int, schoolID *int, event *models.Event) bool {
	if role == models.AdminRoleSuperAdmin {
		return true
	}
	return role == models.AdminRoleSchoolSuperAdmin && schoolID != nil && event.Level != nil && *event.Level == "school" && event.SchoolID != nil && *event.SchoolID == *schoolID
}

func (s *AdminServer) upsertEventManager(ctx echo.Context, event *models.Event, req adminEventRequest) error {
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
	if !canManageEventManager(adminRole(ctx), adminSchoolID(ctx), event) {
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
		result, err := s.repo.DB().ExecContext(ctx.Request().Context(), `INSERT INTO admin_user(username,password_hash,nickname,role,school_id,status) VALUES(?,?,?,?,?,1)`, account, string(hash), nickname, models.AdminRoleEventManager, event.SchoolID)
		if err != nil {
			return response.BadRequest(ctx, "赛事管理员账号已存在")
		}
		id, _ := result.LastInsertId()
		if _, err = s.repo.DB().ExecContext(ctx.Request().Context(), `UPDATE event SET admin_id=? WHERE id=? AND admin_id IS NULL`, id, event.ID); err != nil {
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
		hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		sets = append(sets, "password_hash=?")
		args = append(args, string(hash))
	}
	args = append(args, *event.AdminID)
	if _, err := s.repo.DB().ExecContext(ctx.Request().Context(), `UPDATE admin_user SET `+strings.Join(sets, ",")+`,updated_at=CURRENT_TIMESTAMP WHERE id=? AND role=4`, args...); err != nil {
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
		t, err := time.Parse("2006-01-02", strings.TrimSpace(*req.RegistrationDeadline))
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
