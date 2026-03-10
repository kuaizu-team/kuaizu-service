package service

import (
	"context"
	"fmt"

	"github.com/trv3wood/kuaizu-server/api"
	"github.com/trv3wood/kuaizu-server/internal/models"
	"github.com/trv3wood/kuaizu-server/internal/repository"
)

// ProjectService handles project-related business logic.
type ProjectService struct {
	repo         *repository.Repository
	contentAudit *ContentAuditService
}

// NewProjectService creates a new ProjectService.
func NewProjectService(repo *repository.Repository, contentAudit *ContentAuditService) *ProjectService {
	return &ProjectService{repo: repo, contentAudit: contentAudit}
}

// ProjectListResult holds a page of projects with pagination info.
type ProjectListResult struct {
	List       []models.Project
	Total      int64
	TotalPages int
	Page       int
	Size       int
}

// normalizePageParams enforces sane defaults for page/size.
func normalizePageParams(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 10
	}
	return page, size
}

// ListProjects returns a paginated list of projects with optional filters.
func (s *ProjectService) ListProjects(ctx context.Context, params repository.ListParams) (*ProjectListResult, error) {
	params.Page, params.Size = normalizePageParams(params.Page, params.Size)

	projects, total, err := s.repo.Project.List(ctx, params)
	if err != nil {
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

	projects, total, err := s.repo.Project.List(ctx, params)
	if err != nil {
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

// GetProject retrieves a project by ID and asynchronously increments its view count.
func (s *ProjectService) GetProject(ctx context.Context, id int) (*models.Project, error) {
	project, err := s.repo.Project.GetByID(ctx, id)
	if err != nil {
		return nil, ErrInternal("获取项目详情失败")
	}
	if project == nil {
		return nil, ErrNotFound("项目不存在")
	}

	// Increment view count (fire and forget)
	go func() {
		_ = s.repo.Project.IncrementViewCount(ctx, id)
	}()

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
		return nil, ErrBadRequest("内容包含违规信息，请修改后重试")
	}

	project := &models.Project{
		CreatorID:            input.CreatorID,
		Name:                 input.Name,
		Description:          &input.Description,
		SchoolID:             input.SchoolID,
		MemberCount:          &input.MemberCount,
		Status:               0, // 待审核
		PromotionStatus:      0,
		ViewCount:            0,
		IsCrossSchool:        &input.IsCrossSchool,
		EducationRequirement: input.EducationRequirement,
		SkillRequirement:     input.SkillRequirement,
	}

	if input.Direction != nil {
		direction := int(*input.Direction)
		project.Direction = &direction
	}

	if err := s.repo.Project.Create(ctx, project); err != nil {
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
		return nil, ErrInternal("检查权限失败")
	}
	if !isOwner {
		return nil, ErrForbidden("只有队长可以修改项目")
	}

	// Get existing project
	project, err := s.repo.Project.GetByID(ctx, id)
	if err != nil {
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
			return nil, ErrBadRequest("内容包含违规信息，请修改后重试")
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
		project.Direction = (*int)(input.Direction)
	}
	if input.MemberCount != nil {
		project.MemberCount = input.MemberCount
	}
	if input.IsCrossSchool != nil {
		project.IsCrossSchool = input.IsCrossSchool
	}
	if input.EducationRequirement != nil {
		project.EducationRequirement = input.EducationRequirement
	}
	if input.SkillRequirement != nil {
		project.SkillRequirement = input.SkillRequirement
	}

	if err := s.repo.Project.Update(ctx, project); err != nil {
		return nil, ErrInternal("更新项目失败")
	}

	// Reload to return fresh data
	updated, err := s.repo.Project.GetByID(ctx, id)
	if err != nil {
		return nil, ErrInternal("获取项目信息失败")
	}

	return updated, nil
}

// DeleteProject checks ownership and deletes the project.
func (s *ProjectService) DeleteProject(ctx context.Context, id, userID int) error {
	isOwner, err := s.repo.Project.IsOwner(ctx, id, userID)
	if err != nil {
		return ErrInternal("检查权限失败")
	}
	if !isOwner {
		return ErrForbidden("只有队长可以删除项目")
	}

	if err := s.repo.Project.Delete(ctx, id); err != nil {
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
		return nil, ErrInternal("检查权限失败")
	}
	if !isOwner {
		return nil, ErrForbidden("只有队长可以查看申请列表")
	}

	params.ProjectID = &projectID

	applications, total, err := s.repo.Application.List(ctx, params)
	if err != nil {
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
	Contact   *string
}

// ApplyToProject validates and creates a project application.
func (s *ProjectService) ApplyToProject(ctx context.Context, input ApplyToProjectInput) (*models.ProjectApplication, error) {
	project, err := s.repo.Project.GetByID(ctx, input.ProjectID)
	if err != nil {
		return nil, ErrInternal("获取项目信息失败")
	}
	if project == nil {
		return nil, ErrNotFound("项目不存在")
	}

	if project.CreatorID == input.UserID {
		return nil, ErrBadRequest("不能申请加入自己的项目")
	}

	if project.Status != 1 {
		return nil, ErrBadRequest("该项目当前不接受申请")
	}

	exists, err := s.repo.Application.CheckDuplicate(ctx, input.ProjectID, input.UserID)
	if err != nil {
		return nil, ErrInternal("检查申请状态失败")
	}
	if exists {
		return nil, ErrBadRequest("您已申请过该项目")
	}

	application := &models.ProjectApplication{
		ProjectID: input.ProjectID,
		UserID:    input.UserID,
		Contact:   input.Contact,
		Status:    0, // 待审核
	}

	if err := s.repo.Application.Create(ctx, application); err != nil {
		return nil, ErrInternal("提交申请失败")
	}

	return application, nil
}

// ReviewApplication validates and updates the status of a project application.
func (s *ProjectService) ReviewApplication(ctx context.Context, applicationID, userID int, status api.ApplicationStatus) error {
	if status != api.ApplicationStatusN1 && status != api.ApplicationStatusN2 {
		return ErrBadRequest(fmt.Sprintf("无效的审核状态: %d", status))
	}

	app, err := s.repo.Application.GetByID(ctx, applicationID)
	if err != nil {
		return ErrInternal("获取申请信息失败")
	}
	if app == nil {
		return ErrNotFound("申请不存在")
	}

	isOwner, err := s.repo.Project.IsOwner(ctx, app.ProjectID, userID)
	if err != nil {
		return ErrInternal("检查权限失败")
	}
	if !isOwner {
		return ErrForbidden("只有队长可以审核申请")
	}

	if err := s.repo.Application.UpdateStatus(ctx, applicationID, int(status)); err != nil {
		return ErrInternal("更新申请状态失败")
	}

	// TODO: Send notification to applicant

	return nil
}
