package handler

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/auth"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/service"
	"github.com/labstack/echo/v4"
)

type createProjectRequest struct {
	api.CreateProjectDTO
	EventIDs *[]int `json:"eventIds"`
}

type updateProjectRequest struct {
	api.UpdateProjectDTO
	EventIDs *[]int `json:"eventIds"`
}

// ListProjects handles GET /projects
func (s *Server) ListProjects(ctx echo.Context, params api.ListProjectsParams) error {
	listParams := repository.ListParams{
		Page:           1,
		Size:           10,
		Keyword:        params.Keyword,
		SchoolID:       params.SchoolId,
		EventID:        params.EventId,
		ExcludeEventID: params.ExcludeEventId,
	}
	if params.EventIds != nil {
		if len(*params.EventIds) > 10 {
			return BadRequest(ctx, "最多同时筛选10个赛事")
		}
		for _, eventID := range *params.EventIds {
			if eventID <= 0 {
				return BadRequest(ctx, "赛事ID必须为正整数")
			}
		}
		listParams.EventIDs = append([]int(nil), (*params.EventIds)...)
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
	viewerUserID := getProjectListViewerUserID(ctx)
	if viewerUserID > 0 {
		listParams.ViewerUserID = &viewerUserID
	}
	listParams.RandomSeed = fmt.Sprintf("%d:%s", viewerUserID, time.Now().Format("2006-01-02"))
	if params.RandomSeed != nil && *params.RandomSeed != "" {
		listParams.RandomSeed = *params.RandomSeed
	}

	result, err := s.svc.Project.ListProjects(ctx.Request().Context(), listParams)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	ids := make([]int, len(result.List))
	for i := range result.List {
		ids[i] = result.List[i].ID
	}
	interactions, err := s.repo.Interaction.Batch(ctx.Request().Context(), repository.InteractionProject, ids, GetOptionalUserID(ctx))
	if err != nil {
		return InternalError(ctx, "get project interactions failed")
	}
	for i := range result.List {
		result.List[i].Interaction = interactions[result.List[i].ID]
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

// GET /projects remains public, so the global JWT middleware skips it. Parse a
// valid optional bearer token locally for personalized ranking; missing or
// invalid credentials intentionally retain anonymous random ordering.
func getProjectListViewerUserID(ctx echo.Context) int {
	if userID := GetOptionalUserID(ctx); userID > 0 {
		return userID
	}
	authHeader := strings.TrimSpace(ctx.Request().Header.Get("Authorization"))
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
		return 0
	}
	claims, err := auth.ParseToken(auth.DefaultConfig(), strings.TrimSpace(parts[1]))
	if err != nil {
		return 0
	}
	return claims.UserID
}

// CreateProject handles POST /projects
func (s *Server) CreateProject(ctx echo.Context) error {
	userID := GetUserID(ctx)

	var req createProjectRequest
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
		Tags:                 req.Tags,
		PublisherRole:        req.PublisherRole,
		InitiatingSchoolID:   req.InitiatingSchoolId,
		Milestones:           req.Milestones,
		ImageKeys:            req.ImageKeys,
		Members:              req.Members,
		EventIDs:             req.EventIDs,
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
	ids := make([]int, len(result.List))
	for i := range result.List {
		ids[i] = result.List[i].ID
	}
	unread, err := s.repo.Interaction.BatchProjectUnread(ctx.Request().Context(), userID, ids)
	if err != nil {
		return InternalError(ctx, "get project dashboard unread failed")
	}
	for i := range result.List {
		count := unread[result.List[i].ID]
		result.List[i].InteractionUnreadCount = &count
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
	recordView := true
	if params.RecordView != nil {
		recordView = *params.RecordView
	}

	project, err := s.svc.Project.GetProjectDetail(ctx.Request().Context(), id, userID, source, recordView)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	interaction, err := s.repo.Interaction.Get(ctx.Request().Context(), repository.InteractionProject, id, userID)
	if err != nil {
		return InternalError(ctx, "get project interactions failed")
	}
	project.Interaction = *interaction

	return Success(ctx, project.ToDetailVO())
}

// GetProjectDashboard handles GET /projects/{id}/dashboard
func (s *Server) GetProjectDashboard(ctx echo.Context, id int, params api.GetProjectDashboardParams) error {
	userID := GetUserID(ctx)

	if id <= 0 {
		return BadRequest(ctx, "无效的项目 ID")
	}

	days := 30
	if params.Days != nil {
		days = *params.Days
	}
	result, err := s.svc.Project.GetProjectDashboard(ctx.Request().Context(), id, userID, days)
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
	page := 1
	if params.Page != nil && *params.Page > 0 {
		page = int(*params.Page)
	}

	result, err := s.svc.Project.GetProjectViewers(ctx.Request().Context(), id, userID, page, limit)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	return Success(ctx, map[string]any{
		"total": result.Total,
		"list":  result.List,
	})
}

// ListProjectPromotionBatches handles GET /projects/{id}/promotion-batches.
func (s *Server) ListProjectPromotionBatches(ctx echo.Context, id int, params api.ListProjectPromotionBatchesParams) error {
	userID := GetUserID(ctx)

	if id <= 0 {
		return BadRequest(ctx, "invalid project id")
	}

	page := 1
	if params.Page != nil {
		page = int(*params.Page)
	}
	size := 10
	if params.Size != nil {
		size = int(*params.Size)
	}
	days := 0
	if params.Days != nil {
		days = *params.Days
	}
	limit := 0
	if params.Limit != nil {
		limit = *params.Limit
		if params.Size == nil && limit > 0 {
			size = limit
		}
	}

	result, err := s.svc.EmailPromotion.ListProjectBatchesPaged(ctx.Request().Context(), userID, id, page, size, days, 0)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	return Success(ctx, result)
}

// ListProjectPromotionBatchUsers handles GET /projects/{id}/promotion-batches/{batchId}/users.
func (s *Server) ListProjectPromotionBatchUsers(ctx echo.Context, id int, batchId int, params api.ListProjectPromotionBatchUsersParams) error {
	userID := GetUserID(ctx)

	if id <= 0 || batchId <= 0 {
		return BadRequest(ctx, "invalid project or batch id")
	}

	page := 1
	if params.Page != nil {
		page = int(*params.Page)
	}
	size := 20
	if params.Size != nil {
		size = int(*params.Size)
	}

	result, err := s.svc.EmailPromotion.ListProjectBatchUsers(ctx.Request().Context(), userID, id, batchId, page, size)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	return Success(ctx, result)
}

// UpdateProject handles PUT /projects/{id}
func (s *Server) UpdateProject(ctx echo.Context, id int) error {
	userID := GetUserID(ctx)

	var req updateProjectRequest
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
		Tags:                 req.Tags,
		PublisherRole:        req.PublisherRole,
		SchoolID:             req.SchoolId,
		InitiatingSchoolID:   req.InitiatingSchoolId,
		Milestones:           req.Milestones,
		ImageKeys:            req.ImageKeys,
		Members:              req.Members,
		EventIDs:             req.EventIDs,
	}

	project, err := s.svc.Project.UpdateProject(ctx.Request().Context(), id, userID, input)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	for _, key := range project.RemovedImageKeys {
		if err := s.svc.Commons.DeleteFile(key); err != nil {
			log.Printf("[UpdateProject] delete removed project image failed: %v", err)
			continue
		}
		_ = s.repo.Media.CompleteCleanup(ctx.Request().Context(), key)
	}

	return Success(ctx, project.ToVO())
}

// SubmitMilestoneCertification handles the manually registered certification endpoint.
func (s *Server) SubmitMilestoneCertification(ctx echo.Context, projectID int, milestoneID int) error {
	if projectID <= 0 {
		return BadRequest(ctx, "无效的项目 ID")
	}
	if milestoneID <= 0 {
		return BadRequest(ctx, "无效的时间节点 ID")
	}
	var req struct {
		EvidenceKeys []string `json:"evidenceKeys"`
	}
	if err := ctx.Bind(&req); err != nil {
		return BadRequest(ctx, "请求参数错误")
	}
	userID := GetUserID(ctx)
	removed, err := s.repo.Media.SubmitMilestoneEvidence(ctx.Request().Context(), userID, projectID, milestoneID, req.EvidenceKeys)
	if err != nil {
		return BadRequest(ctx, "佐证图片无效或无权认证该节点")
	}
	for _, key := range removed {
		if err := s.svc.Commons.DeleteFile(key); err != nil {
			log.Printf("[SubmitMilestoneCertification] delete replaced evidence failed: %v", err)
			continue
		}
		_ = s.repo.Media.CompleteCleanup(ctx.Request().Context(), key)
	}
	return Success(ctx, map[string]interface{}{"certificationStatus": 1})
}

// DeleteProject handles DELETE /projects/{id}
func (s *Server) DeleteProject(ctx echo.Context, id int) error {
	userID := GetUserID(ctx)

	if err := s.svc.Project.DeleteProject(ctx.Request().Context(), id, userID); err != nil {
		return mapServiceError(ctx, err)
	}

	return SuccessMessage(ctx, "项目已删除")
}

func (s *Server) ListProjectMemberRatings(ctx echo.Context, id int) error {
	result, err := s.svc.Project.ListProjectMemberRatings(ctx.Request().Context(), id, GetUserID(ctx))
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return Success(ctx, result)
}

func (s *Server) RateProjectMember(ctx echo.Context, id int) error {
	var req api.RateProjectMemberJSONRequestBody
	if err := ctx.Bind(&req); err != nil {
		return BadRequest(ctx, "请求参数错误")
	}
	result, err := s.svc.Project.RateProjectMember(
		ctx.Request().Context(), id, GetUserID(ctx), req.TargetUserId, req.Score,
	)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return Success(ctx, result)
}

func (s *Server) RemoveProjectMember(ctx echo.Context, id int, memberId int, params api.RemoveProjectMemberParams) error {
	userID := GetUserID(ctx)
	result, err := s.svc.Project.RemoveMember(ctx.Request().Context(), id, userID, memberId, params.Score)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return Success(ctx, result)
}

func (s *Server) RestoreProject(ctx echo.Context, id int) error {
	userID := GetUserID(ctx)
	if id <= 0 {
		return BadRequest(ctx, "invalid project id")
	}
	if err := s.svc.Project.RestoreProjectByUser(ctx.Request().Context(), id, userID); err != nil {
		return mapServiceError(ctx, err)
	}
	return Success(ctx, nil)
}

func (s *Server) CompleteRecruit(ctx echo.Context, id int) error {
	userID := GetUserID(ctx)
	if id <= 0 {
		return BadRequest(ctx, "invalid project id")
	}
	if err := s.svc.Project.CompleteRecruit(ctx.Request().Context(), id, userID); err != nil {
		return mapServiceError(ctx, err)
	}
	return Success(ctx, nil)
}

func (s *Server) RestartRecruit(ctx echo.Context, id int) error {
	userID := GetUserID(ctx)
	if id <= 0 {
		return BadRequest(ctx, "invalid project id")
	}
	if err := s.svc.Project.RestartRecruit(ctx.Request().Context(), id, userID); err != nil {
		return mapServiceError(ctx, err)
	}
	return Success(ctx, nil)
}

func (s *Server) EndProject(ctx echo.Context, id int) error {
	userID := GetUserID(ctx)
	if id <= 0 {
		return BadRequest(ctx, "invalid project id")
	}
	if err := s.svc.Project.EndProject(ctx.Request().Context(), id, userID); err != nil {
		return mapServiceError(ctx, err)
	}
	return Success(ctx, nil)
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

// GetMyProjectStatusUnread handles GET /users/me/project-status-unread.
// Returns whether any passive project review status changed since the user last viewed the page.
func (s *Server) GetMyProjectStatusUnread(ctx echo.Context) error {
	userID := GetUserID(ctx)

	hasUnread, err := s.repo.Project.HasUnreadPassiveStatusChange(ctx.Request().Context(), userID)
	if err != nil {
		return InternalError(ctx, "获取项目状态未读失败")
	}

	return Success(ctx, map[string]bool{"hasUnread": hasUnread})
}

// MarkMyProjectStatusRead handles POST /users/me/project-status-read.
// Sets last_viewed_my_projects_at = NOW() so that the passive status red dot resets.
func (s *Server) MarkMyProjectStatusRead(ctx echo.Context) error {
	userID := GetUserID(ctx)

	if err := s.repo.User.UpdateLastViewedMyProjectsAt(ctx.Request().Context(), userID); err != nil {
		return InternalError(ctx, "标记项目状态已读失败")
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

	log.Printf("mark reviewer application read request: userID=%d projectID=%d ids=%v", userID, req.ProjectId, req.Ids)
	if err := s.repo.Application.MarkReviewerRead(ctx.Request().Context(), req.ProjectId, userID, req.Ids); err != nil {
		if errors.Is(err, repository.ErrNotProjectOwner) {
			return Forbidden(ctx, "无权操作")
		}
		return InternalError(ctx, "标记已读失败")
	}

	return Success(ctx, nil)
}

func (s *Server) AssignApplicationRole(ctx echo.Context, id int) error {
	userID := GetUserID(ctx)

	var req struct {
		Role string `json:"role"`
	}
	if err := ctx.Bind(&req); err != nil {
		return BadRequest(ctx, "请求参数错误")
	}
	if err := s.svc.Project.AssignApplicationRole(ctx.Request().Context(), service.AssignApplicationRoleInput{
		ApplicationID: id,
		UserID:        userID,
		Role:          req.Role,
	}); err != nil {
		return mapServiceError(ctx, err)
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

func (s *Server) WithdrawMyApplication(ctx echo.Context, id int) error {
	userID := GetUserID(ctx)
	if id <= 0 {
		return BadRequest(ctx, "无效的申请ID")
	}
	if err := s.svc.Project.WithdrawMyApplication(ctx.Request().Context(), id, userID); err != nil {
		return mapServiceError(ctx, err)
	}
	return Success(ctx, nil)
}
