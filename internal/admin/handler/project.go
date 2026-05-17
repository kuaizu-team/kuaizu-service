package handler

import (
	"fmt"
	"strconv"

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
	if sid := adminSchoolID(ctx); sid != nil {
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

	// 校区管理员只能查看本校项目
	if sid := adminSchoolID(ctx); sid != nil {
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

	// 校区管理员只能操作本校项目
	if sid := adminSchoolID(ctx); sid != nil {
		project, err := s.svc.Project.GetProject(ctx.Request().Context(), id)
		if err != nil {
			return mapServiceError(ctx, err)
		}
		if project == nil || project.SchoolID == nil || *project.SchoolID != *sid {
			return response.Forbidden(ctx, "权限不足")
		}
	}

	if err := s.svc.Project.TakedownProject(ctx.Request().Context(), id); err != nil {
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
	Status int `json:"status"`
}

// ReviewProject handles PATCH /admin/projects/:id
func (s *AdminServer) ReviewProject(ctx echo.Context) error {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid project id")
	}

	// 校区管理员只能操作本校项目
	if sid := adminSchoolID(ctx); sid != nil {
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

	if err := s.svc.Project.ReviewProject(ctx.Request().Context(), id, req.Status); err != nil {
		return mapServiceError(ctx, err)
	}

	return response.SuccessMessage(ctx, "操作成功")
}
