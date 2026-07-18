package handler

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	adminvo "github.com/kuaizu-team/kuaizu-service/internal/admin/vo"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
)

// ListProjects handles GET /admin/projects
func (s *AdminServer) ListProjects(ctx echo.Context) error {
	page, _ := strconv.Atoi(ctx.QueryParam("page"))
	size, _ := strconv.Atoi(ctx.QueryParam("size"))

	params := repository.ListParams{
		Page:                page,
		Size:                size,
		IncludePendingCount: true, // admin list always needs pending count column
	}
	if adminRole(ctx) == models.AdminRoleEventManager {
		eventID, err := s.eventIDForManager(ctx)
		if err != nil {
			return err
		}
		params.EventID = &eventID
	}

	if v := ctx.QueryParam("status"); v != "" {
		status, err := strconv.Atoi(v)
		if err != nil {
			return response.BadRequest(ctx, "invalid status")
		}
		params.Status = &status
	}

	if v := ctx.QueryParam("keyword"); v != "" {
		params.Keyword = &v
	}

	if v := ctx.QueryParam("creatorId"); v != "" {
		creatorID, err := strconv.Atoi(v)
		if err != nil {
			return response.BadRequest(ctx, "invalid creatorId")
		}
		params.CreatorID = &creatorID
	}

	// sortBy / order — unknown values are silently ignored (degraded to default)
	if v := ctx.QueryParam("sortBy"); v != "" {
		params.SortBy = &v
	}
	if v := ctx.QueryParam("order"); v != "" {
		params.Order = &v
	}

	// 校区管理员自动按学校过滤
	if adminRole(ctx) == models.AdminRoleSchoolSuperAdmin {
		schoolIDs, err := s.adminSchoolIDs(ctx)
		if err != nil {
			return response.InternalError(ctx, "查询学校权限失败")
		}
		params.SchoolIDs = schoolIDs
	} else if sid := adminSchoolID(ctx); sid != nil && adminRole(ctx) != models.AdminRoleEventManager {
		params.SchoolID = sid
	}

	result, err := s.svc.Project.ListProjects(ctx.Request().Context(), params)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	list := make([]adminvo.AdminProjectVO, len(result.List))
	for i := range result.List {
		list[i] = *adminvo.NewAdminProjectVO(&result.List[i])
	}

	return response.Success(ctx, map[string]interface{}{
		"list":  list,
		"total": result.Total,
		"page":  result.Page,
		"size":  result.Size,
	})
}

// GetProject handles GET /admin/projects/:id
func (s *AdminServer) GetProject(ctx echo.Context) error {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid project id")
	}

	project, err := s.svc.Project.GetProject(ctx.Request().Context(), id)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	if err := s.requireProjectAccess(ctx, id); err != nil {
		return err
	}

	// 校区管理员只能查看本校项目
	if sid := adminSchoolID(ctx); sid != nil && adminRole(ctx) != models.AdminRoleEventManager {
		if project == nil || project.SchoolID == nil || *project.SchoolID != *sid {
			return response.Forbidden(ctx, "权限不足")
		}
	}

	return response.Success(ctx, adminvo.NewAdminProjectVO(project))
}

// TakedownProject handles PATCH /admin/projects/:id/takedown
func (s *AdminServer) TakedownProject(ctx echo.Context) error {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid project id")
	}
	if err := s.requireProjectAccess(ctx, id); err != nil {
		return err
	}

	// 校区管理员只能操作本校项目
	if sid := adminSchoolID(ctx); sid != nil && adminRole(ctx) != models.AdminRoleEventManager {
		project, err := s.svc.Project.GetProject(ctx.Request().Context(), id)
		if err != nil {
			return mapServiceError(ctx, err)
		}
		if project == nil || project.SchoolID == nil || *project.SchoolID != *sid {
			return response.Forbidden(ctx, "权限不足")
		}
	}

	var req reviewProjectRequest
	_ = ctx.Bind(&req)
	reason := strings.TrimSpace(req.RejectReason)
	if reason == "" {
		return response.BadRequest(ctx, "rejectReason is required")
	}
	if err := s.svc.Project.TakedownProject(ctx.Request().Context(), id, &reason); err != nil {
		return mapServiceError(ctx, err)
	}

	return response.SuccessMessage(ctx, "操作成功")
}

// ListProjectApplications handles GET /admin/projects/:id/applications
func (s *AdminServer) ListProjectApplications(ctx echo.Context) error {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid project id")
	}
	if err := s.requireProjectAccess(ctx, id); err != nil {
		return err
	}

	page, _ := strconv.Atoi(ctx.QueryParam("page"))
	size, _ := strconv.Atoi(ctx.QueryParam("size"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 10
	}

	applications, total, err := s.repo.Application.List(ctx.Request().Context(), repository.ApplicationListParams{
		ProjectID: &id,
		Page:      page,
		Size:      size,
	})
	if err != nil {
		return response.InternalError(ctx, "获取投递记录失败")
	}

	// Batch-query talent_profile status for all applicant users
	talentStatusMap := make(map[int]int, len(applications))
	if len(applications) > 0 {
		userIDs := make([]int, 0, len(applications))
		for _, a := range applications {
			if a.Applicant != nil {
				userIDs = append(userIDs, a.Applicant.ID)
			}
		}
		if len(userIDs) > 0 {
			type tpStatusRow struct {
				UserID int `db:"user_id"`
				Status int `db:"status"`
			}
			q, args, qErr := sqlx.In(`SELECT user_id, status FROM talent_profile WHERE user_id IN (?)`, userIDs)
			if qErr == nil {
				q = s.repo.DB().Rebind(q)
				var rows []tpStatusRow
				if sErr := s.repo.DB().SelectContext(ctx.Request().Context(), &rows, q, args...); sErr == nil {
					for _, row := range rows {
						talentStatusMap[row.UserID] = row.Status
					}
				}
			}
		}
	}

	list := make([]adminvo.AdminProjectApplicantVO, len(applications))
	for i := range applications {
		var talentStatus *int
		if applications[i].Applicant != nil {
			if status, ok := talentStatusMap[applications[i].Applicant.ID]; ok {
				s := status
				talentStatus = &s
			}
		}
		list[i] = *adminvo.NewAdminProjectApplicantVO(&applications[i], talentStatus)
	}

	return response.Success(ctx, map[string]interface{}{
		"list":  list,
		"total": total,
	})
}

// ListProjectOliveBranches handles GET /admin/projects/:id/olive-branches
func (s *AdminServer) ListProjectOliveBranches(ctx echo.Context) error {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid project id")
	}
	if err := s.requireProjectAccess(ctx, id); err != nil {
		return err
	}

	page, _ := strconv.Atoi(ctx.QueryParam("page"))
	size, _ := strconv.Atoi(ctx.QueryParam("size"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 10
	}

	branches, total, err := s.repo.OliveBranch.ListByRelatedProjectID(ctx.Request().Context(), repository.OliveBranchByProjectParams{
		ProjectID: id,
		Page:      page,
		Size:      size,
	})
	if err != nil {
		return response.InternalError(ctx, "获取橄榄枝记录失败")
	}

	// Batch-query talent_profile status for all receiver users
	talentStatusMap := make(map[int]int, len(branches))
	if len(branches) > 0 {
		userIDs := make([]int, 0, len(branches))
		for _, ob := range branches {
			if ob.Receiver != nil {
				userIDs = append(userIDs, ob.Receiver.ID)
			}
		}
		if len(userIDs) > 0 {
			type tpStatusRow struct {
				UserID int `db:"user_id"`
				Status int `db:"status"`
			}
			q, args, qErr := sqlx.In(`SELECT user_id, status FROM talent_profile WHERE user_id IN (?)`, userIDs)
			if qErr == nil {
				q = s.repo.DB().Rebind(q)
				var rows []tpStatusRow
				if sErr := s.repo.DB().SelectContext(ctx.Request().Context(), &rows, q, args...); sErr == nil {
					for _, row := range rows {
						talentStatusMap[row.UserID] = row.Status
					}
				}
			}
		}
	}

	list := make([]adminvo.AdminProjectOliveBranchVO, len(branches))
	for i := range branches {
		var talentStatus *int
		if branches[i].Receiver != nil {
			if status, ok := talentStatusMap[branches[i].Receiver.ID]; ok {
				s := status
				talentStatus = &s
			}
		}
		list[i] = *adminvo.NewAdminProjectOliveBranchVO(&branches[i], talentStatus)
	}

	return response.Success(ctx, map[string]interface{}{
		"list":  list,
		"total": total,
	})
}

type reviewProjectRequest struct {
	Status       int    `json:"status"`
	RejectReason string `json:"rejectReason"`
}

type createProjectMilestoneRequest struct {
	MilestoneDate string `json:"milestoneDate"`
	Description   string `json:"description"`
}

type updateProjectMemberRoleRequest struct {
	Role string `json:"role"`
}
type replaceProjectEventsRequest struct {
	EventIDs []int `json:"eventIds"`
}
type updateProjectAdminNoteRequest struct {
	AdminNote *string `json:"adminNote"`
}
type updateProjectLifecycleStatusRequest struct {
	Status int `json:"status"`
}

func (s *AdminServer) CreateProjectMilestone(ctx echo.Context) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}
	id, err := parseIDParam(ctx, "id", "project")
	if err != nil {
		return err
	}
	var req createProjectMilestoneRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	date, err := time.Parse("2006-01-02", strings.TrimSpace(req.MilestoneDate))
	description := strings.TrimSpace(req.Description)
	if err != nil || description == "" {
		return response.BadRequest(ctx, "时间和描述不能为空")
	}
	result, err := s.repo.DB().ExecContext(ctx.Request().Context(), `INSERT INTO project_milestones(project_id,milestone_date,description,sort_order) SELECT ?,?,?,COALESCE(MAX(sort_order),0)+1 FROM project_milestones WHERE project_id=?`, id, date, description, id)
	if err != nil {
		return response.InternalError(ctx, "新增时间节点失败")
	}
	milestoneID, _ := result.LastInsertId()
	return response.Success(ctx, map[string]interface{}{"id": milestoneID, "milestoneDate": req.MilestoneDate, "description": description})
}

func (s *AdminServer) UpdateProjectMemberRole(ctx echo.Context) error {
	if role := adminRole(ctx); role != models.AdminRoleSuperAdmin && role != models.AdminRoleSchoolSuperAdmin {
		return response.Forbidden(ctx, "权限不足")
	}
	projectID, err := parseIDParam(ctx, "id", "project")
	if err != nil {
		return err
	}
	if err := s.requireProjectAccess(ctx, projectID); err != nil {
		return err
	}
	memberID, err := parseIDParam(ctx, "memberId", "member")
	if err != nil {
		return err
	}
	var req updateProjectMemberRoleRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	role := strings.TrimSpace(req.Role)
	var roleExists bool
	if err := s.repo.DB().QueryRowxContext(ctx.Request().Context(), "SELECT EXISTS(SELECT 1 FROM project_role WHERE code=?)", role).Scan(&roleExists); err != nil || !roleExists {
		return response.BadRequest(ctx, "无效的成员角色")
	}
	result, err := s.repo.DB().ExecContext(ctx.Request().Context(), "UPDATE project_members SET role=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND project_id=?", role, memberID, projectID)
	if err != nil {
		return response.InternalError(ctx, "调整成员角色失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return response.NotFound(ctx, "项目成员不存在")
	}
	return response.SuccessMessage(ctx, "成员角色已更新")
}

func (s *AdminServer) ReplaceProjectEvents(ctx echo.Context) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}
	projectID, err := parseIDParam(ctx, "id", "project")
	if err != nil {
		return err
	}
	var req replaceProjectEventsRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	tx, err := s.repo.DB().BeginTxx(ctx.Request().Context(), nil)
	if err != nil {
		return response.InternalError(ctx, "更新赛事关联失败")
	}
	defer tx.Rollback()
	if err := s.repo.Event.ReplaceProjectEventsTx(ctx.Request().Context(), tx, projectID, req.EventIDs); err != nil {
		return response.BadRequest(ctx, "赛事关联数据无效")
	}
	if err := tx.Commit(); err != nil {
		return response.InternalError(ctx, "更新赛事关联失败")
	}
	return response.SuccessMessage(ctx, "赛事关联已更新")
}

func (s *AdminServer) UpdateProjectAdminNote(ctx echo.Context) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}
	id, err := parseIDParam(ctx, "id", "project")
	if err != nil {
		return err
	}
	var req updateProjectAdminNoteRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	var note *string
	if req.AdminNote != nil {
		value := strings.TrimSpace(*req.AdminNote)
		if value != "" {
			note = &value
		}
	}
	result, err := s.repo.DB().ExecContext(ctx.Request().Context(), "UPDATE project SET admin_note=?,admin_note_updated_at=CURRENT_TIMESTAMP WHERE id=?", note, id)
	if err != nil {
		return response.InternalError(ctx, "保存项目备注失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return response.NotFound(ctx, "项目不存在")
	}
	return response.SuccessMessage(ctx, "项目备注已保存")
}

func (s *AdminServer) UpdateProjectLifecycleStatus(ctx echo.Context) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}
	id, err := parseIDParam(ctx, "id", "project")
	if err != nil {
		return err
	}
	var req updateProjectLifecycleStatusRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	var result sql.Result
	if req.Status == models.ProjectStatusRecruitCompleted {
		var currentStatus int
		if err := s.repo.DB().QueryRowxContext(ctx.Request().Context(), "SELECT status FROM project WHERE id=?", id).Scan(&currentStatus); err != nil || currentStatus != models.ProjectStatusApproved {
			return response.BadRequest(ctx, "当前项目状态不支持此操作")
		}
		if _, err := s.repo.Project.CompleteRecruit(ctx.Request().Context(), id); err != nil {
			return response.InternalError(ctx, "更新项目状态失败")
		}
		return response.SuccessMessage(ctx, "项目状态已更新")
	} else if req.Status == models.ProjectStatusEnded {
		result, err = s.repo.DB().ExecContext(ctx.Request().Context(), "UPDATE project SET status=?,ended_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=? AND status IN (?,?)", req.Status, id, models.ProjectStatusApproved, models.ProjectStatusRecruitCompleted)
	} else {
		return response.BadRequest(ctx, "status must be 3 or 5")
	}
	if err != nil {
		return response.InternalError(ctx, "更新项目状态失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return response.BadRequest(ctx, "当前项目状态不支持此操作")
	}
	return response.SuccessMessage(ctx, "项目状态已更新")
}

// ReviewProject handles PATCH /admin/projects/:id
func (s *AdminServer) ReviewProject(ctx echo.Context) error {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid project id")
	}
	if err := s.requireProjectAccess(ctx, id); err != nil {
		return err
	}
	if adminRole(ctx) == models.AdminRoleEventManager {
		var currentStatus int
		if err := s.repo.DB().QueryRowxContext(ctx.Request().Context(), "SELECT status FROM project WHERE id=?", id).Scan(&currentStatus); err != nil {
			return response.NotFound(ctx, "项目不存在")
		}
		if currentStatus != models.ProjectStatusPending {
			return response.Forbidden(ctx, "赛事管理员只能审核待审核项目")
		}
	}

	// 校区管理员只能操作本校项目
	if sid := adminSchoolID(ctx); sid != nil && adminRole(ctx) != models.AdminRoleEventManager {
		project, err := s.svc.Project.GetProject(ctx.Request().Context(), id)
		if err != nil {
			return mapServiceError(ctx, err)
		}
		if project == nil || project.SchoolID == nil || *project.SchoolID != *sid {
			return response.Forbidden(ctx, "权限不足")
		}
	}

	var req reviewProjectRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}

	if req.Status != models.ProjectStatusApproved && req.Status != models.ProjectStatusRejected {
		return response.BadRequest(ctx, fmt.Sprintf("invalid status %d, must be %d (approve) or %d (reject)", req.Status, models.ProjectStatusApproved, models.ProjectStatusRejected))
	}

	reason := strings.TrimSpace(req.RejectReason)
	if req.Status == models.ProjectStatusRejected && reason == "" {
		return response.BadRequest(ctx, "rejectReason is required")
	}
	var reasonPtr *string
	if reason != "" {
		reasonPtr = &reason
	}
	if err := s.svc.Project.ReviewProject(ctx.Request().Context(), id, req.Status, reasonPtr); err != nil {
		return mapServiceError(ctx, err)
	}

	return response.SuccessMessage(ctx, "操作成功")
}

func (s *AdminServer) RestoreProject(ctx echo.Context) error {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid project id")
	}
	if err := s.requireProjectAccess(ctx, id); err != nil {
		return err
	}

	if sid := adminSchoolID(ctx); sid != nil && adminRole(ctx) != models.AdminRoleEventManager {
		project, err := s.svc.Project.GetProject(ctx.Request().Context(), id)
		if err != nil {
			return mapServiceError(ctx, err)
		}
		if project == nil || project.SchoolID == nil || *project.SchoolID != *sid {
			return response.Forbidden(ctx, "权限不足")
		}
	}

	if err := s.svc.Project.RestoreProject(ctx.Request().Context(), id); err != nil {
		return mapServiceError(ctx, err)
	}
	return response.SuccessMessage(ctx, "操作成功")
}

// PermanentlyDeleteProject handles DELETE /admin/projects/:id/permanent.
func (s *AdminServer) PermanentlyDeleteProject(ctx echo.Context) error {
	role := adminRole(ctx)
	if role != models.AdminRoleSuperAdmin && role != models.AdminRoleEventManager {
		return response.Forbidden(ctx, "权限不足")
	}
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil || id <= 0 {
		return response.BadRequest(ctx, "invalid project id")
	}
	if err := s.requireProjectAccess(ctx, id); err != nil {
		return err
	}

	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	if _, err := s.repo.PurgeDeletedProjectBefore(ctx.Request().Context(), id, cutoff); err != nil {
		if err == sql.ErrNoRows {
			return response.BadRequest(ctx, "项目不存在或尚未满足永久删除条件")
		}
		return response.InternalError(ctx, "永久删除项目失败")
	}
	return response.SuccessMessage(ctx, "永久删除成功")
}

// GetProjectActivitySummary returns lightweight project activity counts.
func (s *AdminServer) GetProjectActivitySummary(ctx echo.Context) error {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil || id <= 0 {
		return response.BadRequest(ctx, "invalid project id")
	}
	if err := s.requireProjectAccess(ctx, id); err != nil {
		return err
	}
	if sid := adminSchoolID(ctx); sid != nil && adminRole(ctx) != models.AdminRoleEventManager {
		project, err := s.svc.Project.GetProject(ctx.Request().Context(), id)
		if err != nil {
			return mapServiceError(ctx, err)
		}
		if project == nil || project.SchoolID == nil || *project.SchoolID != *sid {
			return response.Forbidden(ctx, "权限不足")
		}
	}
	var summary struct {
		ApplicationsTotal    int `db:"applications_total" json:"applicationsTotal"`
		ApplicationsPending  int `db:"applications_pending" json:"applicationsPending"`
		OliveBranchesTotal   int `db:"olive_branches_total" json:"oliveBranchesTotal"`
		OliveBranchesPending int `db:"olive_branches_pending" json:"oliveBranchesPending"`
	}
	if err := s.repo.DB().GetContext(ctx.Request().Context(), &summary, `
		SELECT
			(SELECT COUNT(*) FROM project_application WHERE project_id = ?) AS applications_total,
			(SELECT COUNT(*) FROM project_application WHERE project_id = ? AND status = 0) AS applications_pending,
			(SELECT COUNT(*) FROM olive_branch_record WHERE related_project_id = ?) AS olive_branches_total,
			(SELECT COUNT(*) FROM olive_branch_record WHERE related_project_id = ? AND status = 0) AS olive_branches_pending
	`, id, id, id, id); err != nil {
		return response.InternalError(ctx, "获取项目业务统计失败")
	}
	return response.Success(ctx, summary)
}
