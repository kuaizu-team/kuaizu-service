package service

import (
	"context"
	"log"

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

// ListMyProjects returns a paginated list of projects created by the given user.
func (s *ProjectService) ListMyProjects(ctx context.Context, userID int, params repository.ListParams) (*ProjectListResult, error) {
	params.Page, params.Size = normalizePageParams(params.Page, params.Size)
	params.CreatorID = &userID

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

// GetProject retrieves a project by ID and asynchronously increments its view count.
func (s *ProjectService) GetProject(ctx context.Context, id int) (*models.Project, error) {
	project, err := s.repo.Project.GetByID(ctx, id)
	if err != nil {
		log.Printf("[ProjectService.GetProject] repository error: %v", err)
		return nil, ErrInternal("获取项目详情失败")
	}
	if project == nil {
		return nil, ErrNotFound("项目不存在")
	}

	// Increment view count (fire and forget)
	go func(asyncCtx context.Context) {
		_ = s.repo.Project.IncrementViewCount(asyncCtx, id)
	}(context.WithoutCancel(ctx))

	return project, nil
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
}

// CreateProject validates input, audits content, and creates a new project.
func (s *ProjectService) CreateProject(ctx context.Context, input CreateProjectInput) (*models.Project, error) {
	if input.Name == "" {
		return nil, ErrBadRequest("项目名称不能为空")
	}
	if input.MemberCount < 1 {
		return nil, ErrBadRequest("需求人数必须大于0")
	}

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

	if err := s.repo.Project.Create(ctx, project); err != nil {
		log.Printf("[ProjectService.CreateProject] repository error: %v", err)
		return nil, ErrInternal("创建项目失败")
	}

	return project, nil
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
}

// UpdateProject checks ownership, audits content, applies updates, and returns the updated project.
func (s *ProjectService) UpdateProject(ctx context.Context, id, userID int, input UpdateProjectInput) (*models.Project, error) {
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

	if err := s.repo.Project.Update(ctx, project); err != nil {
		log.Printf("[ProjectService.UpdateProject] repository error updating: %v", err)
		return nil, ErrInternal("更新项目失败")
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

	// 向项目所有者发送收到名片订阅消息
	go func(asyncCtx context.Context) {
		// 1. 获取申请人信息
		applicant, err := s.repo.User.GetByID(asyncCtx, input.UserID)
		if err != nil {
			log.Printf("[ProjectService.ApplyToProject] error getting applicant: %v", err)
			return
		}

		senderName := "匿名用户"
		if applicant.Nickname != nil {
			senderName = *applicant.Nickname
		}

		// 2. 发送订阅消息
		data := map[string]string{
			"sender":       senderName,
			"project_name": project.Name,
			"remark":       "您收到了新的名片投递，请及时处理。",
		}

		err = s.message.SendSubscribeMsgByBizKey(asyncCtx, project.CreatorID, models.MsgBizKeyCardReceived, data)
		if err != nil {
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
		remark := "恭喜！您已成功加入项目，请主动联系队长。"
		if status == models.ApplicationStatusRejected {
			resultStr = "已拒绝"
			remark = "很抱歉，您的申请未通过。您可以尝试申请其他感兴趣的项目。"
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
		statusStr := "已通过"
		remark := "恭喜！您的项目已通过审核，现在对其他用户可见。"
		if status == models.ProjectStatusRejected {
			statusStr = "已驳回"
			remark = "很抱歉，您的项目未通过审核，请检查内容是否合规。"
		}

		data := map[string]string{
			"project_name": project.Name,
			"status":       statusStr,
			"apply_time":   project.UpdatedAt.Format("2006-01-02 15:04:05"),
			"remark":       remark,
		}

		err = s.message.SendSubscribeMsgByBizKey(asyncCtx, project.CreatorID, models.MsgBizKeyAuditResultProj, data)
		if err != nil {
			log.Printf("[ProjectService.ReviewProject] notification error: %v", err)
		}
	}(context.WithoutCancel(ctx))

	return nil
}
