package handler

import (
	"github.com/labstack/echo/v4"
	"github.com/trv3wood/kuaizu-server/api"
	"github.com/trv3wood/kuaizu-server/internal/repository"
	"github.com/trv3wood/kuaizu-server/internal/service"
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
	if params.Status != nil {
		status := int(*params.Status)
		listParams.Status = &status
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
func (s *Server) GetProject(ctx echo.Context, id int) error {
	project, err := s.svc.Project.GetProject(ctx.Request().Context(), id)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	return Success(ctx, project.ToDetailVO())
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

	var req api.ApplyToProjectJSONBody
	if err := ctx.Bind(&req); err != nil {
		return BadRequest(ctx, "请求参数错误")
	}

	input := service.ApplyToProjectInput{
		ProjectID: id,
		UserID:    userID,
		Contact:   req.Contact,
	}

	application, err := s.svc.Project.ApplyToProject(ctx.Request().Context(), input)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	return Success(ctx, application.ToVO())
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
