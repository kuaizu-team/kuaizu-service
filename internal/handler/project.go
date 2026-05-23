package handler

import (
	"errors"
	"strconv"
	"strings"

	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/service"
	"github.com/labstack/echo/v4"
)

// ListProjects handles GET /projects
func (s *Server) ListProjects(ctx echo.Context, params api.ListProjectsParams) error {
	listParams := repository.ListParams{
		Page:     1,
		Size:     10,
		Keyword:  params.Keyword,
		SchoolID: params.SchoolId,
	}

	if params.Page != nil {
		listParams.Page = *params.Page
	}
	if params.Size != nil {
		listParams.Size = *params.Size
	}
	if params.Status != nil {
		status := int(*params.Status)
		listParams.Status = &status
	}
	if params.Direction != nil {
		direction := int(*params.Direction)
		listParams.Direction = &direction
	}
	if params.IsCrossSchool != nil {
		isCrossSchool := int(*params.IsCrossSchool)
		listParams.IsCrossSchool = &isCrossSchool
	}
	listParams.SortBy = params.SortBy
	listParams.UserSchoolID = params.UserSchoolId

	result, err := s.svc.Project.ListProjects(ctx.Request().Context(), listParams)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	list := make([]api.ProjectVO, len(result.List))
	for i, p := range result.List {
		list[i] = *p.ToVO()
	}

	pageInfo := api.PageInfo{
		Page:       &result.Page,
		Size:       &result.Size,
		Total:      &result.Total,
		TotalPages: &result.TotalPages,
	}

	return Success(ctx, api.ProjectPageResponse{
		List:     &list,
		PageInfo: &pageInfo,
	})
}

// CreateProject handles POST /projects
func (s *Server) CreateProject(ctx echo.Context) error {
	userID := GetUserID(ctx)

	var req api.CreateProjectDTO
	if err := ctx.Bind(&req); err != nil {
		return BadRequest(ctx, "请求参数错误")
	}

	input := service.CreateProjectInput{
		CreatorID:            userID,
		Name:                 req.Name,
		Description:          req.Description,
		SchoolID:             req.SchoolId,
		MemberCount:          req.MemberCount,
		IsCrossSchool:        req.IsCrossSchool,
		Direction:            req.Direction,
		EducationRequirement: req.EducationRequirement,
		SkillRequirement:     req.SkillRequirement,
	}

	project, err := s.svc.Project.CreateProject(ctx.Request().Context(), input)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	return Success(ctx, project.ToVO())
}

// ListMyProjects handles GET /projects/my
func (s *Server) ListMyProjects(ctx echo.Context, params api.ListMyProjectsParams) error {
	userID := GetUserID(ctx)

	listParams := repository.ListParams{
		Page: 1,
		Size: 10,
	}

	if params.Page != nil {
		listParams.Page = *params.Page
	}
	if params.Size != nil {
		listParams.Size = *params.Size
	}
	if params.Status != nil && *params.Status != "" {
		parts := strings.Split(*params.Status, ",")
		statuses := make([]int, 0, len(parts))
		for _, p := range parts {
			if v, err := strconv.Atoi(strings.TrimSpace(p)); err == nil {
				statuses = append(statuses, v)
			}
		}
		if len(statuses) == 1 {
			listParams.Status = &statuses[0]
		}
		listParams.Statuses = statuses
	}

	result, err := s.svc.Project.ListMyProjects(ctx.Request().Context(), userID, listParams)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	list := make([]api.ProjectVO, len(result.List))
	for i, p := range result.List {
		list[i] = *p.ToVO()
	}

	pageInfo := api.PageInfo{
		Page:       &result.Page,
		Size:       &result.Size,
		Total:      &result.Total,
		TotalPages: &result.TotalPages,
	}

	return Success(ctx, api.ProjectPageResponse{
		List:     &list,
		PageInfo: &pageInfo,
	})
}

// GetProject handles GET /projects/{id}
func (s *Server) GetProject(ctx echo.Context, id int, params api.GetProjectParams) error {
	userID := GetUserID(ctx)

	source := 0
	if params.Source != nil {
		source = *params.Source
	}

	project, err := s.svc.Project.GetProjectWithView(ctx.Request().Context(), id, userID, source)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	return Success(ctx, project.ToDetailVO())
}

// GetProjectDashboard handles GET /projects/{id}/dashboard
func (s *Server) GetProjectDashboard(ctx echo.Context, id int) error {
	userID := GetUserID(ctx)

	if id <= 0 {
		return BadRequest(ctx, "无效的项目 ID")
	}

	result, err := s.svc.Project.GetProjectDashboard(ctx.Request().Context(), id, userID)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	return Success(ctx, result)
}

// RecordViewDuration handles POST /projects/{id}/view-duration
// Records a dwell-time entry for the project. Fire-and-forget from the frontend.
func (s *Server) RecordViewDuration(ctx echo.Context, id int) error {
	userID := GetUserID(ctx)

	if id <= 0 {
		return BadRequest(ctx, "无效的项目 ID")
	}

	var req struct {
		DurationMs int `json:"duration_ms"`
	}
	if err := ctx.Bind(&req); err != nil {
		return BadRequest(ctx, "请求参数错误")
	}

	if err := s.svc.Project.RecordViewDuration(ctx.Request().Context(), id, userID, req.DurationMs); err != nil {
		return mapServiceError(ctx, err)
	}

	return Success(ctx, nil)
}

// GetProjectViewers handles GET /projects/{id}/viewers
// Returns authenticated users who viewed the project in the last 24 hours.
func (s *Server) GetProjectViewers(ctx echo.Context, id int, params api.GetProjectViewersParams) error {
	userID := GetUserID(ctx)

	if id <= 0 {
		return BadRequest(ctx, "无效的项目 ID")
	}

	limit := 20
	if params.Limit != nil && *params.Limit > 0 {
		limit = *params.Limit
		if limit > 50 {
			limit = 50
		}
	}

	result, err := s.svc.Project.GetProjectViewers(ctx.Request().Context(), id, userID, limit)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	return Success(ctx, map[string]any{
		"total": result.Total,
		"list":  result.List,
	})
}

// UpdateProject handles PUT /projects/{id}
func (s *Server) UpdateProject(ctx echo.Context, id int) error {
	userID := GetUserID(ctx)

	var req api.UpdateProjectDTO
	if err := ctx.Bind(&req); err != nil {
		return BadRequest(ctx, "请求参数错误")
	}

	input := service.UpdateProjectInput{
		Name:                 req.Name,
		Description:          req.Description,
		Direction:            req.Direction,
		MemberCount:          req.MemberCount,
		IsCrossSchool:        req.IsCrossSchool,
		EducationRequirement: req.EducationRequirement,
		SkillRequirement:     req.SkillRequirement,
		NeedReview:           req.NeedReview,
	}

	project, err := s.svc.Project.UpdateProject(ctx.Request().Context(), id, userID, input)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	return Success(ctx, project.ToVO())
}

// DeleteProject handles DELETE /projects/{id}
func (s *Server) DeleteProject(ctx echo.Context, id int) error {
	userID := GetUserID(ctx)

	if err := s.svc.Project.DeleteProject(ctx.Request().Context(), id, userID); err != nil {
		return mapServiceError(ctx, err)
	}

	return SuccessMessage(ctx, "项目已删除")
}

// ListProjectApplications handles GET /projects/{id}/applications
func (s *Server) ListProjectApplications(ctx echo.Context, id int, params api.ListProjectApplicationsParams) error {
	userID := GetUserID(ctx)

	listParams := repository.ApplicationListParams{
		Page: 1,
		Size: 10,
	}

	if params.Page != nil {
		listParams.Page = *params.Page
	}
	if params.Size != nil {
		listParams.Size = *params.Size
	}
	if params.Status != nil {
		status := int(*params.Status)
		listParams.Status = &status
	}

	result, err := s.svc.Project.ListProjectApplications(ctx.Request().Context(), id, userID, listParams)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	list := make([]api.ProjectApplicationVO, len(result.List))
	for i, app := range result.List {
		list[i] = *app.ToVO()
	}

	pageInfo := api.PageInfo{
		Page:       &result.Page,
		Size:       &result.Size,
		Total:      &result.Total,
		TotalPages: &result.TotalPages,
	}

	return Success(ctx, api.ApplicationPageResponse{
		List:     &list,
		PageInfo: &pageInfo,
	})
}

// ListMyApplications handles GET /applications/my
func (s *Server) ListMyApplications(ctx echo.Context, params api.ListMyApplicationsParams) error {
	userID := GetUserID(ctx)

	listParams := repository.ApplicationListParams{
		Page: 1,
		Size: 10,
	}

	if params.Page != nil {
		listParams.Page = *params.Page
	}
	if params.Size != nil {
		listParams.Size = *params.Size
	}
	if params.Status != nil {
		status := int(*params.Status)
		listParams.Status = &status
	}

	result, err := s.svc.Project.ListMyApplications(ctx.Request().Context(), userID, listParams)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	list := make([]api.ProjectApplicationVO, len(result.List))
	for i, app := range result.List {
		list[i] = *app.ToVO()
	}

	pageInfo := api.PageInfo{
		Page:       &result.Page,
		Size:       &result.Size,
		Total:      &result.Total,
		TotalPages: &result.TotalPages,
	}

	return Success(ctx, api.ApplicationPageResponse{
		List:     &list,
		PageInfo: &pageInfo,
	})
}

// ApplyToProject handles POST /projects/{id}/applications
func (s *Server) ApplyToProject(ctx echo.Context, id int) error {
	userID := GetUserID(ctx)

	input := service.ApplyToProjectInput{
		ProjectID: id,
		UserID:    userID,
	}

	application, err := s.svc.Project.ApplyToProject(ctx.Request().Context(), input)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	return Success(ctx, application.ToVO())
}

// GetMyApplicationUnreadStatus handles GET /project-applications/my/unread-status
// Returns how many of the current user's applications have changed status since the last time they viewed the page.
func (s *Server) GetMyApplicationUnreadStatus(ctx echo.Context) error {
	userID := GetUserID(ctx)

	count, err := s.repo.Application.GetUnreadApplicationCount(ctx.Request().Context(), userID)
	if err != nil {
		return InternalError(ctx, "获取未读申请数失败")
	}

	return Success(ctx, map[string]int{"unreadCount": count})
}

// MarkMyApplicationsRead handles POST /project-applications/my/mark-read
// Sets applications_last_viewed_at = NOW() so that unreadCount resets to 0.
func (s *Server) MarkMyApplicationsRead(ctx echo.Context) error {
	userID := GetUserID(ctx)

	if err := s.repo.User.UpdateApplicationsLastViewedAt(ctx.Request().Context(), userID); err != nil {
		return InternalError(ctx, "标记已读失败")
	}

	return Success(ctx, nil)
}

// MarkReviewerApplicationRead handles POST /project-applications/mark-read
// Called by the project owner when viewing a project's application list.
func (s *Server) MarkReviewerApplicationRead(ctx echo.Context) error {
	userID := GetUserID(ctx)

	var req struct {
		ProjectId int   `json:"projectId"`
		Ids       []int `json:"ids"`
	}
	if err := ctx.Bind(&req); err != nil {
		return BadRequest(ctx, "请求参数错误")
	}
	if req.ProjectId == 0 {
		return BadRequest(ctx, "projectId 不能为空")
	}

	if err := s.repo.Application.MarkReviewerRead(ctx.Request().Context(), req.ProjectId, userID, req.Ids); err != nil {
		if errors.Is(err, repository.ErrNotProjectOwner) {
			return Forbidden(ctx, "无权操作")
		}
		return InternalError(ctx, "标记已读失败")
	}

	return Success(ctx, nil)
}

// ReviewApplication handles PATCH /project-applications/{id}
func (s *Server) ReviewApplication(ctx echo.Context, id int) error {
	userID := GetUserID(ctx)

	var req api.ReviewApplicationJSONBody
	if err := ctx.Bind(&req); err != nil {
		return InvalidParams(ctx, err)
	}

	if err := s.svc.Project.ReviewApplication(ctx.Request().Context(), id, userID, req.Status); err != nil {
		return mapServiceError(ctx, err)
	}

	return Success(ctx, nil)
}
