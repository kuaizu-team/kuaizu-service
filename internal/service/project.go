package service

import (
	"context"
	"log"
	"strings"
	"unicode/utf8"

	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

// ProjectService handles project-related business logic.
type ProjectService struct {
	repo         *repository.Repository
	contentAudit *ContentAuditService
	message      *MessageService
}

type projectMetadataRepo interface {
	CreateWithMetadata(ctx context.Context, p *models.Project, tags *[]string, publisherRole *string, initiatingSchoolID *int) error
	UpdateWithMetadata(ctx context.Context, p *models.Project, tags *[]string, publisherRole *string, initiatingSchoolID *int) error
}

// NewProjectService creates a new ProjectService.
func NewProjectService(repo *repository.Repository, contentAudit *ContentAuditService, message *MessageService) *ProjectService {
	return &ProjectService{repo: repo, contentAudit: contentAudit, message: message}
}

// ProjectListResult holds a page of projects with pagination info.
type ProjectListResult struct {
	List       []models.Project
	Total      int64
	TotalPages int
	Page       int
	Size       int
}

// ListProjects returns a paginated list of projects with optional filters.
func (s *ProjectService) ListProjects(ctx context.Context, params repository.ListParams) (*ProjectListResult, error) {
	params.Page, params.Size = normalizePageParams(params.Page, params.Size)

	if err := validateProjectListStatuses(params); err != nil {
		return nil, err
	}

	// When school_priority sort is requested and a user school ID is provided,
	// look up the school's geo info (province/city/district) so the repository
	// can build the full P1→P5 geo-priority ORDER BY.
	// If the school lookup fails or returns nothing we simply leave the geo fields
	// nil — the repository degrades gracefully to fewer tiers.
	if params.SortBy != nil && *params.SortBy == "school_priority" && params.UserSchoolID != nil {
		school, err := s.repo.School.GetByID(ctx, *params.UserSchoolID)
		if err != nil {
			log.Printf("[ProjectService.ListProjects] school lookup error (non-fatal): %v", err)
		} else if school != nil {
			params.UserSchoolProvince = school.Province
			params.UserSchoolCity = school.City
			params.UserSchoolDistrict = school.District
		}
	}

	projects, total, err := s.repo.Project.List(ctx, params)
	if err != nil {
		log.Printf("[ProjectService.ListProjects] repository error: %v", err)
		return nil, ErrInternal("获取项目列表失败")
	}

	totalPages := int((total + int64(params.Size) - 1) / int64(params.Size))
	return &ProjectListResult{
		List:       projects,
		Total:      total,
		TotalPages: totalPages,
		Page:       params.Page,
		Size:       params.Size,
	}, nil
}

// ListMyProjects returns a paginated list of projects created by the given user,
// sorted by updated_at descending so recently-modified projects appear first.
func (s *ProjectService) ListMyProjects(ctx context.Context, userID int, params repository.ListParams) (*ProjectListResult, error) {
	params.Page, params.Size = normalizePageParams(params.Page, params.Size)
	params.CreatorID = &userID
	sortBy := "updated_at"
	params.SortBy = &sortBy

	if err := validateProjectListStatuses(params); err != nil {
		return nil, err
	}

	projects, total, err := s.repo.Project.List(ctx, params)
	if err != nil {
		log.Printf("[ProjectService.ListMyProjects] repository error: %v", err)
		return nil, ErrInternal("获取我的项目列表失败")
	}

	totalPages := int((total + int64(params.Size) - 1) / int64(params.Size))
	return &ProjectListResult{
		List:       projects,
		Total:      total,
		TotalPages: totalPages,
		Page:       params.Page,
		Size:       params.Size,
	}, nil
}

func validateProjectListStatuses(params repository.ListParams) error {
	if len(params.Statuses) > 0 {
		for _, status := range params.Statuses {
			if err := IsValidStatus("project.status", status); err != nil {
				return err
			}
		}
		return nil
	}

	if params.Status != nil {
		return IsValidStatus("project.status", *params.Status)
	}

	return nil
}

// GetProject retrieves a project by ID without recording a view.
func (s *ProjectService) GetProject(ctx context.Context, id int) (*models.Project, error) {
	project, err := s.repo.Project.GetByID(ctx, id)
	if err != nil {
		log.Printf("[ProjectService.GetProject] repository error: %v", err)
		return nil, ErrInternal("获取项目详情失败")
	}
	if project == nil {
		return nil, ErrNotFound("项目不存在")
	}

	return project, nil
}

// GetProjectWithView retrieves a project and asynchronously records a real user view.
func (s *ProjectService) GetProjectWithView(ctx context.Context, id, viewerUserID, source int) (*models.Project, error) {
	project, err := s.GetProject(ctx, id)
	if err != nil {
		return nil, err
	}

	go func(asyncCtx context.Context) {
		_ = s.repo.Project.IncrementViewCount(asyncCtx, id)
		uid := viewerUserID
		var uidPtr *int
		if uid > 0 {
			uidPtr = &uid
		}
		entry := &models.ProjectViewLog{
			ProjectID: id,
			UserID:    uidPtr,
			Source:    source,
		}
		if err := s.repo.ProjectViewLog.InsertViewLog(asyncCtx, entry); err != nil {
			log.Printf("[ProjectService.GetProject] view log error (non-fatal): %v", err)
		}
	}(context.WithoutCancel(ctx))

	return project, nil
}

// ProjectDashboardResult is the response payload for GET /projects/{id}/dashboard.
type ProjectDashboardResult struct {
	TotalViews         int                          `json:"total_views"`
	TodayViews         int                          `json:"today_views"`
	TotalApplicants    int                          `json:"total_applicants"`
	ConversionRate     float64                      `json:"conversion_rate"`
	AvgDurationSeconds int                          `json:"avg_duration_seconds"`
	HourlyViews        []repository.HourlyViewItem  `json:"hourly_views"`
	LikeCount          int                          `json:"like_count"`
	FavoriteCount      int                          `json:"favorite_count"`
	ShareCount         int                          `json:"share_count"`
	InteractionUnread  repository.InteractionUnread `json:"interaction_unread"`
	SourceStats        struct {
		FromList  int `json:"from_list"`
		FromShare int `json:"from_share"`
		Unknown   int `json:"unknown"`
	} `json:"source_stats"`
}

// GetProjectDashboard returns aggregated stats for the project dashboard.
// Only the project creator (requesterUserID == project.creator_id) may access this.
func (s *ProjectService) GetProjectDashboard(ctx context.Context, projectID, requesterUserID, days int) (*ProjectDashboardResult, error) {
	isOwner, err := s.repo.Project.IsOwner(ctx, projectID, requesterUserID)
	if err != nil {
		log.Printf("[ProjectService.GetProjectDashboard] ownership check error: %v", err)
		return nil, ErrInternal("检查权限失败")
	}
	if !isOwner {
		return nil, ErrForbidden("只有项目队长可以查看数据看板")
	}

	raw, err := s.repo.ProjectViewLog.GetDashboardStats(ctx, projectID)
	if err != nil {
		log.Printf("[ProjectService.GetProjectDashboard] stats query error: %v", err)
		return nil, ErrInternal("获取看板数据失败")
	}

	result := &ProjectDashboardResult{
		TotalViews:         raw.TotalViews,
		TodayViews:         raw.TodayViews,
		TotalApplicants:    raw.TotalApplicants,
		ConversionRate:     raw.ConversionRate,
		AvgDurationSeconds: raw.AvgDurationSeconds,
		HourlyViews:        raw.HourlyViews,
	}
	result.SourceStats.FromList = raw.FromList
	result.SourceStats.FromShare = raw.FromShare
	result.SourceStats.Unknown = raw.Unknown
	counts, err := s.repo.Interaction.CountsSince(ctx, repository.InteractionProject, projectID, days)
	if err != nil {
		return nil, ErrInternal("get interaction dashboard failed")
	}
	result.LikeCount, result.FavoriteCount, result.ShareCount = counts.LikeCount, counts.FavoriteCount, counts.ShareCount
	unread, err := s.repo.Interaction.UnreadForTarget(ctx, repository.InteractionProject, projectID, requesterUserID)
	if err != nil {
		return nil, ErrInternal("get interaction unread failed")
	}
	result.InteractionUnread = unread

	return result, nil
}

// RecordViewDuration inserts a dwell-time entry for a project.
// Invalid or extreme values are silently ignored to keep the endpoint fire-and-forget.
func (s *ProjectService) RecordViewDuration(ctx context.Context, projectID, viewerUserID, durationMs int) error {
	if durationMs <= 0 || durationMs > 3_600_000 {
		return nil
	}
	var uidPtr *int
	if viewerUserID > 0 {
		uid := viewerUserID
		uidPtr = &uid
	}
	if err := s.repo.ProjectViewLog.InsertDurationLog(ctx, projectID, uidPtr, durationMs); err != nil {
		log.Printf("[ProjectService.RecordViewDuration] error: %v", err)
		return ErrInternal("上报停留时长失败")
	}
	return nil
}

// ViewersResult is the payload for GET /projects/{id}/viewers.
type ViewersResult struct {
	Total int
	List  []repository.ProjectViewer
}

// GetProjectViewers returns authenticated viewers of the project in the last 24 h.
// Only the project creator may call this endpoint.
func (s *ProjectService) GetProjectViewers(ctx context.Context, projectID, requesterUserID, limit int) (*ViewersResult, error) {
	isOwner, err := s.repo.Project.IsOwner(ctx, projectID, requesterUserID)
	if err != nil {
		log.Printf("[ProjectService.GetProjectViewers] ownership check error: %v", err)
		return nil, ErrInternal("检查权限失败")
	}
	if !isOwner {
		return nil, ErrForbidden("只有项目队长可以查看访客记录")
	}

	viewers, total, err := s.repo.ProjectViewLog.GetViewers(ctx, projectID, limit)
	if err != nil {
		log.Printf("[ProjectService.GetProjectViewers] query error: %v", err)
		return nil, ErrInternal("获取访客记录失败")
	}

	return &ViewersResult{Total: total, List: viewers}, nil
}

// CreateProjectInput is the DTO for creating a project.
type CreateProjectInput struct {
	CreatorID            int
	Name                 string
	Description          string
	SchoolID             *int
	MemberCount          int
	IsCrossSchool        int
	Direction            *api.Direction
	EducationRequirement *int
	SkillRequirement     *string
	Tags                 *[]string
	PublisherRole        *string
	InitiatingSchoolID   *int
}

func validateProjectTags(tags *[]string) error {
	if tags == nil {
		return nil
	}
	if len(*tags) < 1 || len(*tags) > 5 {
		return ErrBadRequest("tags must contain 1-5 items")
	}
	seen := map[string]struct{}{}
	for i := range *tags {
		(*tags)[i] = strings.TrimSpace((*tags)[i])
		if (*tags)[i] == "" || utf8.RuneCountInString((*tags)[i]) > 12 {
			return ErrBadRequest("each tag must contain 1-12 characters")
		}
		if _, ok := seen[(*tags)[i]]; ok {
			return ErrBadRequest("tags must be unique")
		}
		seen[(*tags)[i]] = struct{}{}
	}
	return nil
}

func (s *ProjectService) resolveProjectSchool(ctx context.Context, creatorID int, schoolID, initiatingSchoolID *int, useCreatorDefault bool) (*int, error) {
	effectiveSchoolID := schoolID
	if effectiveSchoolID == nil {
		effectiveSchoolID = initiatingSchoolID
	}
	if effectiveSchoolID == nil && useCreatorDefault {
		user, err := s.repo.User.GetByID(ctx, creatorID)
		if err != nil {
			return nil, ErrInternal("get creator failed")
		}
		if user != nil {
			effectiveSchoolID = user.SchoolID
		}
	}
	if effectiveSchoolID == nil {
		return nil, ErrBadRequest("project school is required; current user has no school")
	}
	school, err := s.repo.School.GetByID(ctx, *effectiveSchoolID)
	if err != nil {
		return nil, ErrInternal("validate project school failed")
	}
	if school == nil {
		return nil, ErrBadRequest("project school does not exist")
	}
	return effectiveSchoolID, nil
}

// CreateProject validates input, audits content, and creates a new project.
func (s *ProjectService) CreateProject(ctx context.Context, input CreateProjectInput) (*models.Project, error) {
	if input.Name == "" {
		return nil, ErrBadRequest("项目名称不能为空")
	}
	if input.MemberCount < 1 {
		return nil, ErrBadRequest("需求人数必须大于0")
	}
	if err := validateProjectTags(input.Tags); err != nil {
		return nil, err
	}
	effectiveSchoolID, err := s.resolveProjectSchool(ctx, input.CreatorID, input.SchoolID, input.InitiatingSchoolID, true)
	if err != nil {
		return nil, err
	}
	input.SchoolID = effectiveSchoolID
	input.InitiatingSchoolID = effectiveSchoolID

	// 文字内容审核
	auditTexts := []string{input.Name, input.Description}
	if input.SkillRequirement != nil {
		auditTexts = append(auditTexts, *input.SkillRequirement)
	}
	if err := s.contentAudit.CheckText(ctx, auditTexts...); err != nil {
		return nil, err
	}

	project := &models.Project{
		CreatorID:            input.CreatorID,
		Name:                 input.Name,
		Description:          &input.Description,
		SchoolID:             input.SchoolID,
		MemberCount:          &input.MemberCount,
		Status:               models.ProjectStatusPending,
		PromotionStatus:      models.ProjectPromotionNone,
		ViewCount:            0,
		IsCrossSchool:        &input.IsCrossSchool,
		EducationRequirement: input.EducationRequirement,
		SkillRequirement:     input.SkillRequirement,
	}

	if input.Direction != nil {
		if err := IsValidStatus("project.direction", int(*input.Direction)); err != nil {
			return nil, err
		}
		direction := int(*input.Direction)
		project.Direction = &direction
	}

	if input.EducationRequirement != nil {
		if err := IsValidStatus("project.education_requirement", *input.EducationRequirement); err != nil {
			return nil, err
		}
	}

	if input.IsCrossSchool != models.ProjectCrossSchoolNo && input.IsCrossSchool != models.ProjectCrossSchoolYes {
		// Manual check since it's not a pointer in input but we check it in validation.go
		if err := IsValidStatus("project.is_cross_school", input.IsCrossSchool); err != nil {
			return nil, err
		}
	}

	projectRepo, ok := s.repo.Project.(projectMetadataRepo)
	if !ok {
		return nil, ErrInternal("project repository does not support metadata transaction")
	}
	if err := projectRepo.CreateWithMetadata(ctx, project, input.Tags, input.PublisherRole, input.InitiatingSchoolID); err != nil {
		log.Printf("[ProjectService.CreateProject] repository error: %v", err)
		return nil, ErrInternal("创建项目失败")
	}
	created, err := s.repo.Project.GetByID(ctx, project.ID)
	if err != nil {
		return nil, ErrInternal("reload project failed")
	}
	return created, nil
}

// UpdateProjectInput is the DTO for updating a project.
type UpdateProjectInput struct {
	Name                 *string
	Description          *string
	Direction            *api.Direction
	MemberCount          *int
	IsCrossSchool        *int
	EducationRequirement *int
	SkillRequirement     *string
	// NeedReview when true resets the project status to pending (0) so it goes
	// back into the admin review queue. Set by the frontend whenever the user
	// actually modifies content.
	NeedReview         *bool
	Tags               *[]string
	PublisherRole      *string
	SchoolID           *int
	InitiatingSchoolID *int
}

// UpdateProject checks ownership, audits content, applies updates, and returns the updated project.
func (s *ProjectService) UpdateProject(ctx context.Context, id, userID int, input UpdateProjectInput) (*models.Project, error) {
	if err := validateProjectTags(input.Tags); err != nil {
		return nil, err
	}
	// Check ownership
	isOwner, err := s.repo.Project.IsOwner(ctx, id, userID)
	if err != nil {
		log.Printf("[ProjectService.UpdateProject] repository error checking ownership: %v", err)
		return nil, ErrInternal("检查权限失败")
	}
	if !isOwner {
		return nil, ErrForbidden("只有队长可以修改项目")
	}

	// Get existing project
	project, err := s.repo.Project.GetByID(ctx, id)
	if err != nil {
		log.Printf("[ProjectService.UpdateProject] repository error getting project: %v", err)
		return nil, ErrInternal("获取项目信息失败")
	}
	if project == nil {
		return nil, ErrNotFound("项目不存在")
	}

	// 文字内容审核
	if input.SchoolID != nil || input.InitiatingSchoolID != nil {
		effectiveSchoolID, err := s.resolveProjectSchool(ctx, userID, input.SchoolID, input.InitiatingSchoolID, false)
		if err != nil {
			return nil, err
		}
		input.SchoolID = effectiveSchoolID
		input.InitiatingSchoolID = effectiveSchoolID
		project.SchoolID = effectiveSchoolID
	}

	var auditTexts []string
	if input.Name != nil {
		auditTexts = append(auditTexts, *input.Name)
	}
	if input.Description != nil {
		auditTexts = append(auditTexts, *input.Description)
	}
	if input.SkillRequirement != nil {
		auditTexts = append(auditTexts, *input.SkillRequirement)
	}
	if len(auditTexts) > 0 {
		if err := s.contentAudit.CheckText(ctx, auditTexts...); err != nil {
			return nil, err
		}
	}

	// Apply updates
	if input.Name != nil {
		project.Name = *input.Name
	}
	if input.Description != nil {
		project.Description = input.Description
	}
	if input.Direction != nil {
		if err := IsValidStatus("project.direction", int(*input.Direction)); err != nil {
			return nil, err
		}
		project.Direction = (*int)(input.Direction)
	}
	if input.MemberCount != nil {
		project.MemberCount = input.MemberCount
	}
	if input.IsCrossSchool != nil {
		if err := IsValidStatus("project.is_cross_school", *input.IsCrossSchool); err != nil {
			return nil, err
		}
		project.IsCrossSchool = input.IsCrossSchool
	}
	if input.EducationRequirement != nil {
		if err := IsValidStatus("project.education_requirement", *input.EducationRequirement); err != nil {
			return nil, err
		}
		project.EducationRequirement = input.EducationRequirement
	}
	if input.SkillRequirement != nil {
		project.SkillRequirement = input.SkillRequirement
	}

	projectRepo, ok := s.repo.Project.(projectMetadataRepo)
	if !ok {
		return nil, ErrInternal("project repository does not support metadata transaction")
	}
	if err := projectRepo.UpdateWithMetadata(ctx, project, input.Tags, input.PublisherRole, input.InitiatingSchoolID); err != nil {
		log.Printf("[ProjectService.UpdateProject] repository error updating: %v", err)
		return nil, ErrInternal("更新项目失败")
	}

	// If the caller signals that content changed, reset status to pending so the
	// project re-enters the admin review queue.
	if input.NeedReview != nil && *input.NeedReview {
		if err := s.repo.Project.UpdateStatus(ctx, id, models.ProjectStatusPending); err != nil {
			log.Printf("[ProjectService.UpdateProject] repository error resetting status: %v", err)
			return nil, ErrInternal("重置审核状态失败")
		}
	}

	// Reload to return fresh data
	updated, err := s.repo.Project.GetByID(ctx, id)
	if err != nil {
		log.Printf("[ProjectService.UpdateProject] repository error reloading: %v", err)
		return nil, ErrInternal("获取项目信息失败")
	}

	return updated, nil
}

// DeleteProject checks ownership and deletes the project.
func (s *ProjectService) DeleteProject(ctx context.Context, id, userID int) error {
	isOwner, err := s.repo.Project.IsOwner(ctx, id, userID)
	if err != nil {
		log.Printf("[ProjectService.DeleteProject] repository error checking ownership: %v", err)
		return ErrInternal("检查权限失败")
	}
	if !isOwner {
		return ErrForbidden("只有队长可以删除项目")
	}

	if err := s.repo.Project.Delete(ctx, id); err != nil {
		log.Printf("[ProjectService.DeleteProject] repository error: %v", err)
		return ErrInternal("删除项目失败")
	}

	return nil
}

// ApplicationListResult holds a page of applications with pagination info.
type ApplicationListResult struct {
	List       []models.ProjectApplication
	Total      int64
	TotalPages int
	Page       int
	Size       int
}

// ListProjectApplications returns paginated applications for a project (owner only).
func (s *ProjectService) ListProjectApplications(ctx context.Context, projectID, userID int, params repository.ApplicationListParams) (*ApplicationListResult, error) {
	params.Page, params.Size = normalizePageParams(params.Page, params.Size)

	// Only the project owner may view applications
	isOwner, err := s.repo.Project.IsOwner(ctx, projectID, userID)
	if err != nil {
		log.Printf("[ProjectService.ListProjectApplications] repository error checking ownership: %v", err)
		return nil, ErrInternal("检查权限失败")
	}
	if !isOwner {
		return nil, ErrForbidden("只有队长可以查看申请列表")
	}

	params.ProjectID = &projectID

	applications, total, err := s.repo.Application.List(ctx, params)
	if err != nil {
		log.Printf("[ProjectService.ListProjectApplications] repository error: %v", err)
		return nil, ErrInternal("获取申请列表失败")
	}

	totalPages := int((total + int64(params.Size) - 1) / int64(params.Size))
	return &ApplicationListResult{
		List:       applications,
		Total:      total,
		TotalPages: totalPages,
		Page:       params.Page,
		Size:       params.Size,
	}, nil
}

// ListMyApplications returns paginated applications submitted by the user.
func (s *ProjectService) ListMyApplications(ctx context.Context, userID int, params repository.ApplicationListParams) (*ApplicationListResult, error) {
	params.Page, params.Size = normalizePageParams(params.Page, params.Size)
	params.UserID = &userID

	applications, total, err := s.repo.Application.List(ctx, params)
	if err != nil {
		log.Printf("[ProjectService.ListMyApplications] repository error: %v", err)
		return nil, ErrInternal("获取申请列表失败")
	}

	totalPages := int((total + int64(params.Size) - 1) / int64(params.Size))
	return &ApplicationListResult{
		List:       applications,
		Total:      total,
		TotalPages: totalPages,
		Page:       params.Page,
		Size:       params.Size,
	}, nil
}

// ApplyToProjectInput is the DTO for submitting a project application.
type ApplyToProjectInput struct {
	ProjectID int
	UserID    int
}

// ApplyToProject validates and creates a project application.
func (s *ProjectService) ApplyToProject(ctx context.Context, input ApplyToProjectInput) (*models.ProjectApplication, error) {
	project, err := s.repo.Project.GetByID(ctx, input.ProjectID)
	if err != nil {
		log.Printf("[ProjectService.ApplyToProject] repository error getting project: %v", err)
		return nil, ErrInternal("获取项目信息失败")
	}
	if project == nil {
		return nil, ErrNotFound("项目不存在")
	}

	if project.CreatorID == input.UserID {
		return nil, ErrBadRequest("不能申请加入自己的项目")
	}

	if project.Status != models.ProjectStatusApproved {
		return nil, ErrBadRequest("该项目当前不接受申请")
	}

	exists, err := s.repo.Application.CheckDuplicate(ctx, input.ProjectID, input.UserID)
	if err != nil {
		log.Printf("[ProjectService.ApplyToProject] repository error checking duplicate: %v", err)
		return nil, ErrInternal("检查申请状态失败")
	}
	if exists {
		return nil, ErrBadRequest("您已申请过该项目")
	}

	application := &models.ProjectApplication{
		ProjectID: input.ProjectID,
		UserID:    input.UserID,
		Status:    models.ApplicationStatusPending,
	}

	if err := s.repo.Application.Create(ctx, application); err != nil {
		log.Printf("[ProjectService.ApplyToProject] repository error creating application: %v", err)
		return nil, ErrInternal("提交申请失败")
	}

	// 向项目所有者发送「收到名片通知」
	go func(asyncCtx context.Context) {
		// 1. 获取申请人信息
		applicant, err := s.repo.User.GetByID(asyncCtx, input.UserID)
		if err != nil {
			log.Printf("[ProjectService.ApplyToProject] error getting applicant: %v", err)
			return
		}

		senderName := "匿名用户"
		if applicant.Nickname != nil && *applicant.Nickname != "" {
			senderName = *applicant.Nickname
		}

		// 2. 发送订阅消息，跳转页面由数据库 page_path 字段决定
		data := map[string]string{
			"sender": senderName,
			"remark": "恭喜，请在我的项目中及时处理哦。",
		}
		if err := s.message.SendSubscribeMsgByBizKey(asyncCtx, project.CreatorID, models.MsgBizKeyCardReceived, data); err != nil {
			log.Printf("[ProjectService.ApplyToProject] notification error: %v", err)
		}
	}(context.WithoutCancel(ctx))

	return application, nil
}

// ReviewApplication validates and updates the status of a project application.
func (s *ProjectService) ReviewApplication(ctx context.Context, applicationID, userID int, status api.ApplicationStatus) error {
	if err := IsValidStatus("application.status", int(status)); err != nil {
		return err
	}

	app, err := s.repo.Application.GetByID(ctx, applicationID)
	if err != nil {
		log.Printf("[ProjectService.ReviewApplication] repository error getting application: %v", err)
		return ErrInternal("获取申请信息失败")
	}
	if app == nil {
		return ErrNotFound("申请不存在")
	}

	isOwner, err := s.repo.Project.IsOwner(ctx, app.ProjectID, userID)
	if err != nil {
		log.Printf("[ProjectService.ReviewApplication] repository error checking ownership: %v", err)
		return ErrInternal("检查权限失败")
	}
	if !isOwner {
		return ErrForbidden("只有队长可以审核申请")
	}

	if err := s.repo.Application.UpdateStatus(ctx, applicationID, int(status)); err != nil {
		log.Printf("[ProjectService.ReviewApplication] repository error updating status: %v", err)
		return ErrInternal("更新申请状态失败")
	}

	// 向申请人发送名片投递结果通知
	go func(asyncCtx context.Context) {
		// 1. 获取项目信息以拿到名称
		project, err := s.repo.Project.GetByID(asyncCtx, app.ProjectID)
		if err != nil {
			log.Printf("[ProjectService.ReviewApplication] error getting project: %v", err)
			return
		}

		// 2. 准备通知数据
		resultStr := "已通过"
		remark := "请在名片-投递名片管理及时处理哦。"
		if status == models.ApplicationStatusRejected {
			resultStr = "被拒绝"
			remark = "别灰心，更多优质校园项目待您探索。"
		}

		data := map[string]string{
			"project_name":    project.Name,
			"delivery_result": resultStr,
			"remark":          remark,
		}

		// 3. 发送消息给申请人 (app.UserID)
		err = s.message.SendSubscribeMsgByBizKey(asyncCtx, app.UserID, models.MsgBizKeyCardDeliveryResult, data)
		if err != nil {
			log.Printf("[ProjectService.ReviewApplication] notification error: %v", err)
		}
	}(context.WithoutCancel(ctx))

	return nil
}

// TakedownProject (admin only) sets an approved project to closed/taken-down.
func (s *ProjectService) TakedownProject(ctx context.Context, id int) error {
	project, err := s.repo.Project.GetByID(ctx, id)
	if err != nil {
		log.Printf("[ProjectService.TakedownProject] repository error: %v", err)
		return ErrInternal("获取项目失败")
	}
	if project == nil {
		return ErrNotFound("项目不存在")
	}
	if project.Status != models.ProjectStatusApproved {
		return ErrBadRequest("只有上线中的项目可以下架")
	}

	if err := s.repo.Project.UpdateStatus(ctx, id, models.ProjectStatusRejected); err != nil {
		log.Printf("[ProjectService.TakedownProject] repository error updating status: %v", err)
		return ErrInternal("下架失败")
	}

	return nil
}

// ReviewProject (admin only) updates project status and notifies creator.
func (s *ProjectService) ReviewProject(ctx context.Context, id, status int) error {
	project, err := s.repo.Project.GetByID(ctx, id)
	if err != nil {
		log.Printf("[ProjectService.ReviewProject] repository error: %v", err)
		return ErrInternal("获取项目失败")
	}
	if project == nil {
		return ErrNotFound("项目不存在")
	}

	if err := s.repo.Project.UpdateStatus(ctx, id, status); err != nil {
		log.Printf("[ProjectService.ReviewProject] repository error updating status: %v", err)
		return ErrInternal("审核失败")
	}

	// 向项目负责人发送审核结果通知
	go func(asyncCtx context.Context) {
		statusStr := "审核通过"
		remark := "项目已上线，快去查看吧！" // 12 字 ≤ thing7 上限 20 字
		if status == models.ProjectStatusRejected {
			statusStr = "审核拒绝"
			remark = "请按照审核意见重新提交项目。" // 14 字 ≤ thing7 上限 20 字
		}

		data := map[string]string{
			"project_name": truncate20(project.Name), // thing15 ≤ 20 字
			"status":       statusStr,
			"remark":       remark,
		}

		err = s.message.SendSubscribeMsgByBizKey(asyncCtx, project.CreatorID, models.MsgBizKeyAuditResultProj, data)
		if err != nil {
			log.Printf("[ProjectService.ReviewProject] notification error: %v", err)
		}
	}(context.WithoutCancel(ctx))

	return nil
}
