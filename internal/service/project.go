package service

import (
	"context"
	"log"
	"math"
	"sort"
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
	CreateWithMetadata(ctx context.Context, p *models.Project, tags *[]string, publisherRole *string, initiatingSchoolID *int, milestones *[]models.ProjectMilestone, members *[]models.ProjectMember, eventIDs *[]int) error
	UpdateWithMetadata(ctx context.Context, p *models.Project, tags *[]string, publisherRole *string, initiatingSchoolID *int, milestones *[]models.ProjectMilestone, members *[]models.ProjectMember, eventIDs *[]int) error
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
// sorted by creation time descending so the newest projects always appear first.
func (s *ProjectService) ListMyProjects(ctx context.Context, userID int, params repository.ListParams) (*ProjectListResult, error) {
	params.Page, params.Size = normalizePageParams(params.Page, params.Size)
	params.MemberUserID = &userID
	sortBy := "createdAt"
	params.SortBy = &sortBy
	if params.Status == nil && len(params.Statuses) == 0 {
		params.Statuses = []int{
			models.ProjectStatusPending,
			models.ProjectStatusApproved,
			models.ProjectStatusRejected,
			models.ProjectStatusRecruitCompleted,
			models.ProjectStatusEnded,
		}
	}

	if err := validateProjectListStatuses(params); err != nil {
		return nil, err
	}

	projects, total, err := s.repo.Project.List(ctx, params)
	if err != nil {
		log.Printf("[ProjectService.ListMyProjects] repository error: %v", err)
		return nil, ErrInternal("获取我的项目列表失败")
	}
	if err := s.attachProjectEvents(ctx, projects); err != nil {
		log.Printf("[ProjectService.ListMyProjects] event enrichment error: %v", err)
		return nil, ErrInternal("get project events failed")
	}
	if err := s.attachProjectPermissions(ctx, projects, userID); err != nil {
		log.Printf("[ProjectService.ListMyProjects] permission enrichment error: %v", err)
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
	milestones, err := s.repo.Project.ListMilestones(ctx, id)
	if err != nil {
		return nil, ErrInternal("获取项目时间线失败")
	}
	members, err := s.repo.Project.ListMembers(ctx, id)
	if err != nil {
		return nil, ErrInternal("获取项目成员失败")
	}
	project.Milestones = milestones
	project.Members = members
	projects := []models.Project{*project}
	if err := s.attachProjectEvents(ctx, projects); err != nil {
		log.Printf("[ProjectService.GetProject] event enrichment error: %v", err)
		return nil, ErrInternal("get project events failed")
	}
	*project = projects[0]

	return project, nil
}

// GetProjectWithView retrieves a project and asynchronously records a real user view.
func (s *ProjectService) GetProjectWithView(ctx context.Context, id, viewerUserID, source int) (*models.Project, error) {
	project, err := s.GetProject(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.attachProjectPermission(ctx, project, viewerUserID); err != nil {
		log.Printf("[ProjectService.GetProjectWithView] permission enrichment error: %v", err)
		return nil, ErrInternal("获取项目详情失败")
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
			return
		}
		if viewerUserID <= 0 || viewerUserID == project.CreatorID {
			return
		}
		if s.message == nil {
			return
		}
		progress, err := s.repo.ProjectViewLog.NotifyProgress(asyncCtx, id, viewerUserID, project.CreatorID)
		if err != nil {
			log.Printf("[ProjectService.GetProjectWithView] get visit notify progress error (non-fatal): %v", err)
			return
		}
		if !shouldSendGroupedInteractionNotification(progress) {
			return
		}
		viewer, err := s.repo.User.GetByID(asyncCtx, viewerUserID)
		if err != nil {
			log.Printf("[ProjectService.GetProjectWithView] get viewer error (non-fatal): %v", err)
		}
		notification, ok := buildProjectVisitNotification(viewerUserID, project, notificationUserName(viewer), progress.DistinctUserCount)
		if ok {
			sendSubscribeNotification(asyncCtx, s.message, notification)
		}
	}(context.WithoutCancel(ctx))

	return project, nil
}

func isProjectLeaderRole(role string) bool {
	return strings.TrimSpace(role) != "" && role != models.ProjectRoleTeamMember
}

func canOperateAsHighestRole(currentRole *string, members []models.ProjectMember) bool {
	if currentRole == nil || strings.TrimSpace(*currentRole) == "" {
		return false
	}
	role := strings.TrimSpace(*currentRole)
	hasTeamLeader := false
	hasOtherLeader := false
	hasTeamMember := false
	for _, member := range members {
		switch member.Role {
		case models.ProjectRoleTeamLeader:
			hasTeamLeader = true
		case models.ProjectRoleTeamMember:
			hasTeamMember = true
		default:
			if isProjectLeaderRole(member.Role) {
				hasOtherLeader = true
			}
		}
	}
	if hasTeamLeader {
		return role == models.ProjectRoleTeamLeader
	}
	if hasOtherLeader {
		return isProjectLeaderRole(role)
	}
	if hasTeamMember {
		return role == models.ProjectRoleTeamMember
	}
	return role != ""
}

func projectRolePriority(role string) int {
	switch strings.TrimSpace(role) {
	case models.ProjectRoleTeamLeader:
		return 1
	case models.ProjectRoleTeamMember, "":
		return 3
	default:
		return 2
	}
}

func canReviewApplicationByRole(currentUserID int, currentRole *string, app *models.ProjectApplication) bool {
	if currentRole == nil || strings.TrimSpace(*currentRole) == "" || app == nil {
		return false
	}
	if app.Status == models.ApplicationStatusPending {
		return true
	}
	if app.ReviewerID != nil && *app.ReviewerID == currentUserID {
		return true
	}
	if app.ReviewerRole == nil || strings.TrimSpace(*app.ReviewerRole) == "" {
		return true
	}
	currentPriority := projectRolePriority(*currentRole)
	reviewerPriority := projectRolePriority(*app.ReviewerRole)
	if currentPriority == 1 {
		return true
	}
	return currentPriority < reviewerPriority
}

func canAssignProjectRole(currentRole *string, assignedRole string) bool {
	if currentRole == nil || strings.TrimSpace(*currentRole) == "" || strings.TrimSpace(assignedRole) == "" {
		return false
	}
	return projectRolePriority(assignedRole) >= projectRolePriority(*currentRole)
}

func canSelfUpdateProjectRole(userID int, nextRole string, members []models.ProjectMember) bool {
	if userID <= 0 || strings.TrimSpace(nextRole) == "" {
		return false
	}
	currentRole := ""
	highestPriority := 3
	for _, member := range members {
		priority := projectRolePriority(member.Role)
		if priority < highestPriority {
			highestPriority = priority
		}
		if member.UserID == userID {
			currentRole = member.Role
		}
	}
	if currentRole == "" {
		return false
	}
	currentPriority := projectRolePriority(currentRole)
	nextPriority := projectRolePriority(nextRole)
	if currentPriority == highestPriority {
		return nextPriority >= highestPriority
	}
	return nextPriority > highestPriority
}

func currentUserProjectRole(project *models.Project, userID int, members []models.ProjectMember) (*string, *string) {
	if userID <= 0 {
		return nil, nil
	}
	for _, member := range members {
		if member.UserID == userID {
			role := member.Role
			return &role, member.RoleName
		}
	}
	if project.CreatorID == userID {
		role := models.ProjectRoleTeamLeader
		roleName := project.PublisherRoleName
		if project.PublisherRole != nil && strings.TrimSpace(*project.PublisherRole) != "" {
			role = strings.TrimSpace(*project.PublisherRole)
		}
		return &role, roleName
	}
	return nil, nil
}

func (s *ProjectService) attachProjectPermission(ctx context.Context, project *models.Project, userID int) error {
	if project == nil {
		return nil
	}
	members := project.Members
	if len(members) == 0 {
		var err error
		members, err = s.repo.Project.ListMembers(ctx, project.ID)
		if err != nil {
			return err
		}
	}
	currentRole, currentRoleName := currentUserProjectRole(project, userID, members)
	canOperate := canOperateAsHighestRole(currentRole, members)
	project.CurrentUserRole = currentRole
	project.CurrentUserRoleName = currentRoleName
	project.CanCompleteRecruitment = &canOperate
	project.CanDeleteMembers = &canOperate
	return nil
}

func (s *ProjectService) attachProjectPermissions(ctx context.Context, projects []models.Project, userID int) error {
	for i := range projects {
		if err := s.attachProjectPermission(ctx, &projects[i], userID); err != nil {
			return err
		}
	}
	return nil
}

func (s *ProjectService) attachProjectEvents(ctx context.Context, projects []models.Project) error {
	if len(projects) == 0 || s.repo.Event == nil {
		return nil
	}
	ids := make([]int, 0, len(projects))
	for i := range projects {
		ids = append(ids, projects[i].ID)
	}
	events, err := s.repo.Event.ListByProjectIDs(ctx, ids)
	if err != nil {
		return err
	}
	for i := range projects {
		projects[i].Events = events[projects[i].ID]
	}
	return nil
}

// ProjectDashboardResult is the response payload for GET /projects/{id}/dashboard.
type ProjectDashboardResult struct {
	TotalViews             int                                       `json:"total_views"`
	TodayViews             int                                       `json:"today_views"`
	UniqueVisitors         int                                       `json:"unique_visitors"`
	TotalApplicants        int                                       `json:"total_applicants"`
	ProcessedApplicants    int                                       `json:"processed_applicants"`
	ConversionRate         float64                                   `json:"conversion_rate"`
	ApplicationRate        float64                                   `json:"application_rate"`
	ApplicationProcessRate float64                                   `json:"application_process_rate"`
	AvgDurationSeconds     int                                       `json:"avg_duration_seconds"`
	HourlyViews            []repository.HourlyViewItem               `json:"hourly_views"`
	LikeCount              int                                       `json:"like_count"`
	FavoriteCount          int                                       `json:"favorite_count"`
	ShareCount             int                                       `json:"share_count"`
	VisitCount             int                                       `json:"visit_count"`
	InteractionUnread      repository.InteractionUnread              `json:"interaction_unread"`
	VisitUnreadCount       int                                       `json:"visit_unread_count"`
	OliveSentTotal         int                                       `json:"olive_sent_total"`
	OliveReadCount         int                                       `json:"olive_read_count"`
	OliveAcceptedCount     int                                       `json:"olive_accepted_count"`
	OliveReadRate          float64                                   `json:"olive_read_rate"`
	OliveAgreeRate         float64                                   `json:"olive_agree_rate"`
	PromotionStats         repository.ProjectPromotionDashboardStats `json:"promotion_stats"`
	SourceStats            struct {
		FromList  int `json:"from_list"`
		FromShare int `json:"from_share"`
		Unknown   int `json:"unknown"`
	} `json:"source_stats"`
}

func dashboardRate(numerator, denominator int) float64 {
	if denominator <= 0 {
		return 0
	}
	return math.Round(float64(numerator)/float64(denominator)*10000) / 100
}

// GetProjectDashboard returns aggregated stats for the project dashboard.
// Project creators and members may access this.
func (s *ProjectService) GetProjectDashboard(ctx context.Context, projectID, requesterUserID, days int) (*ProjectDashboardResult, error) {
	isReviewer, err := s.repo.Project.IsOwnerOrMember(ctx, projectID, requesterUserID)
	if err != nil {
		log.Printf("[ProjectService.GetProjectDashboard] ownership check error: %v", err)
		return nil, ErrInternal("检查权限失败")
	}
	if !isReviewer {
		return nil, ErrForbidden("无权查看数据看板")
	}

	raw, err := s.repo.ProjectViewLog.GetDashboardStats(ctx, projectID)
	if err != nil {
		log.Printf("[ProjectService.GetProjectDashboard] stats query error: %v", err)
		return nil, ErrInternal("获取看板数据失败")
	}

	result := &ProjectDashboardResult{
		TotalViews:             raw.TotalViews,
		TodayViews:             raw.TodayViews,
		UniqueVisitors:         raw.UniqueVisitors,
		TotalApplicants:        raw.TotalApplicants,
		ProcessedApplicants:    raw.ProcessedApplicants,
		ConversionRate:         raw.ConversionRate,
		ApplicationRate:        raw.ConversionRate,
		ApplicationProcessRate: dashboardRate(raw.ProcessedApplicants, raw.TotalApplicants),
		AvgDurationSeconds:     raw.AvgDurationSeconds,
		HourlyViews:            raw.HourlyViews,
		VisitCount:             raw.TotalViews,
		PromotionStats:         raw.PromotionStats,
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
	visitUnread, err := s.repo.ProjectViewLog.CountUnreadVisits(ctx, projectID, requesterUserID)
	if err != nil {
		return nil, ErrInternal("get visit unread failed")
	}
	result.VisitUnreadCount = visitUnread
	result.InteractionUnread.VisitCount = visitUnread

	oliveStats, err := s.repo.OliveBranch.GetProjectDashboardStats(ctx, projectID)
	if err != nil {
		return nil, ErrInternal("get olive branch dashboard failed")
	}
	result.OliveSentTotal = oliveStats.Total
	result.OliveReadCount = oliveStats.Read
	result.OliveAcceptedCount = oliveStats.Accepted
	result.OliveReadRate = dashboardRate(oliveStats.Read, oliveStats.Total)
	result.OliveAgreeRate = dashboardRate(oliveStats.Accepted, oliveStats.Read)

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
// Project creators and members may call this endpoint.
func (s *ProjectService) GetProjectViewers(ctx context.Context, projectID, requesterUserID, limit int) (*ViewersResult, error) {
	isReviewer, err := s.repo.Project.IsOwnerOrMember(ctx, projectID, requesterUserID)
	if err != nil {
		log.Printf("[ProjectService.GetProjectViewers] ownership check error: %v", err)
		return nil, ErrInternal("检查权限失败")
	}
	if !isReviewer {
		return nil, ErrForbidden("无权查看访客记录")
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
	Milestones           *[]api.ProjectMilestoneDTO
	Members              *[]api.ProjectMemberDTO
	EventIDs             *[]int
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

func (s *ProjectService) buildProjectMilestones(input *[]api.ProjectMilestoneDTO) (*[]models.ProjectMilestone, error) {
	if input == nil {
		return nil, nil
	}
	milestones := make([]models.ProjectMilestone, len(*input))
	for i, item := range *input {
		description := strings.TrimSpace(item.Description)
		if description == "" {
			return nil, ErrBadRequest("milestone description is required")
		}
		if utf8.RuneCountInString(description) > 10 {
			return nil, ErrBadRequest("milestone description must be at most 10 characters")
		}
		milestones[i] = models.ProjectMilestone{
			MilestoneDate: item.MilestoneDate.Time,
			Description:   description,
		}
	}
	sort.SliceStable(milestones, func(i, j int) bool {
		return milestones[i].MilestoneDate.Before(milestones[j].MilestoneDate)
	})
	for i := range milestones {
		milestones[i].SortOrder = i + 1
	}
	return &milestones, nil
}

func (s *ProjectService) buildProjectMembers(ctx context.Context, input *[]api.ProjectMemberDTO, creatorID int, creatorRole *string, ensureCreator bool) (*[]models.ProjectMember, error) {
	if input == nil && !ensureCreator {
		return nil, nil
	}
	membersByUser := map[int]models.ProjectMember{}
	if input != nil {
		for _, item := range *input {
			if item.UserId <= 0 {
				return nil, ErrBadRequest("member userId is required")
			}
			role := strings.TrimSpace(item.Role)
			if role == "" {
				return nil, ErrBadRequest("member role is required")
			}
			if _, exists := membersByUser[item.UserId]; exists {
				return nil, ErrBadRequest("members must be unique")
			}
			membersByUser[item.UserId] = models.ProjectMember{UserID: item.UserId, Role: role}
		}
	}
	if ensureCreator {
		role := models.ProjectRoleTeamLeader
		if creatorRole != nil && strings.TrimSpace(*creatorRole) != "" {
			role = strings.TrimSpace(*creatorRole)
		}
		membersByUser[creatorID] = models.ProjectMember{UserID: creatorID, Role: role}
	}
	members := make([]models.ProjectMember, 0, len(membersByUser))
	roleChecked := map[string]bool{}
	for _, member := range membersByUser {
		if _, ok := roleChecked[member.Role]; !ok {
			exists, err := s.repo.Project.RoleExists(ctx, member.Role)
			if err != nil {
				return nil, ErrInternal("validate project role failed")
			}
			if !exists {
				return nil, ErrBadRequest("project role does not exist")
			}
			roleChecked[member.Role] = true
		}
		user, err := s.repo.User.GetByID(ctx, member.UserID)
		if err != nil {
			return nil, ErrInternal("validate project member failed")
		}
		if user == nil {
			return nil, ErrBadRequest("member user does not exist")
		}
		members = append(members, member)
	}
	sort.SliceStable(members, func(i, j int) bool {
		return members[i].UserID < members[j].UserID
	})
	return &members, nil
}

func projectMembersContainUser(members []models.ProjectMember, userID int) bool {
	for _, member := range members {
		if member.UserID == userID {
			return true
		}
	}
	return false
}

func (s *ProjectService) RemoveMember(ctx context.Context, projectID int, scorerID int, memberID int, score *int) error {
	if projectID <= 0 || memberID <= 0 {
		return ErrBadRequest("invalid project or member id")
	}
	if score != nil && (*score < 0 || *score > 100) {
		return ErrBadRequest("score must be between 0 and 100")
	}
	if scorerID == memberID {
		return ErrBadRequest("不能移除自己")
	}

	project, err := s.repo.Project.GetByID(ctx, projectID)
	if err != nil {
		log.Printf("[ProjectService.RemoveMember] repository error getting project: %v", err)
		return ErrInternal("获取项目失败")
	}
	if project == nil {
		return ErrNotFound("项目不存在")
	}
	members, err := s.repo.Project.ListMembers(ctx, projectID)
	if err != nil {
		log.Printf("[ProjectService.RemoveMember] repository error listing members: %v", err)
		return ErrInternal("获取项目成员失败")
	}
	currentRole, _ := currentUserProjectRole(project, scorerID, members)
	if !canOperateAsHighestRole(currentRole, members) {
		return ErrForbidden("当前角色不能移除项目成员")
	}

	found := false
	for _, member := range members {
		if member.UserID == memberID {
			found = true
			break
		}
	}
	if !found {
		return ErrNotFound("项目成员不存在")
	}

	tx, err := s.repo.DB().BeginTxx(ctx, nil)
	if err != nil {
		return ErrInternal("开启事务失败")
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, "DELETE FROM project_members WHERE project_id=? AND user_id=?", projectID, memberID)
	if err != nil {
		log.Printf("[ProjectService.RemoveMember] delete member failed: %v", err)
		return ErrInternal("移除项目成员失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrNotFound("项目成员不存在")
	}

	if score != nil {
		if _, err := tx.ExecContext(ctx, `INSERT INTO collaboration_score(user_id, project_id, scorer_id, score, created_at)
			VALUES (?, ?, ?, ?, NOW())`, memberID, projectID, scorerID, *score); err != nil {
			log.Printf("[ProjectService.RemoveMember] insert score failed: %v", err)
			return ErrInternal("记录协作评分失败")
		}
		if _, err := tx.ExecContext(ctx, `UPDATE `+"`user`"+` SET collaboration_score=(
			SELECT avg_score FROM (SELECT COALESCE(AVG(score), 100) AS avg_score FROM collaboration_score WHERE user_id=?) t
		) WHERE id=?`, memberID, memberID); err != nil {
			log.Printf("[ProjectService.RemoveMember] update user score failed: %v", err)
			return ErrInternal("更新协作指数失败")
		}
	}

	if err := tx.Commit(); err != nil {
		return ErrInternal("提交事务失败")
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
	milestones, err := s.buildProjectMilestones(input.Milestones)
	if err != nil {
		return nil, err
	}
	members, err := s.buildProjectMembers(ctx, input.Members, input.CreatorID, input.PublisherRole, true)
	if err != nil {
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
	if err := projectRepo.CreateWithMetadata(ctx, project, input.Tags, input.PublisherRole, input.InitiatingSchoolID, milestones, members, input.EventIDs); err != nil {
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
	Milestones         *[]api.ProjectMilestoneDTO
	Members            *[]api.ProjectMemberDTO
	EventIDs           *[]int
}

// UpdateProject checks ownership, audits content, applies updates, and returns the updated project.
func (s *ProjectService) UpdateProject(ctx context.Context, id, userID int, input UpdateProjectInput) (*models.Project, error) {
	if err := validateProjectTags(input.Tags); err != nil {
		return nil, err
	}

	project, err := s.repo.Project.GetByID(ctx, id)
	if err != nil {
		log.Printf("[ProjectService.UpdateProject] repository error getting project: %v", err)
		return nil, ErrInternal("获取项目信息失败")
	}
	if project == nil {
		return nil, ErrNotFound("项目不存在")
	}

	isOwner := project.CreatorID == userID
	memberRole, err := s.repo.Project.GetMemberRole(ctx, id, userID)
	if err != nil {
		log.Printf("[ProjectService.UpdateProject] repository error checking member role: %v", err)
		return nil, ErrInternal("检查权限失败")
	}
	if !isOwner && memberRole == nil {
		return nil, ErrForbidden("无权修改项目")
	}

	milestones, err := s.buildProjectMilestones(input.Milestones)
	if err != nil {
		return nil, err
	}
	members, err := s.buildProjectMembers(ctx, input.Members, 0, nil, false)
	if err != nil {
		return nil, err
	}
	if members != nil && !projectMembersContainUser(*members, userID) {
		return nil, ErrBadRequest("不能删除自己")
	}

	if !isOwner {
		if input.Milestones != nil {
			return nil, ErrForbidden("只有项目创建者可以修改项目时间线")
		}
		if input.Members == nil {
			return nil, ErrForbidden("项目成员只能修改成员列表")
		}
		existingMembers, err := s.repo.Project.ListMembers(ctx, id)
		if err != nil {
			return nil, ErrInternal("获取项目成员失败")
		}
		if canOperateAsHighestRole(memberRole, existingMembers) {
			if err := s.repo.Project.ReplaceMembers(ctx, id, *members); err != nil {
				log.Printf("[ProjectService.UpdateProject] repository error replacing members: %v", err)
				return nil, ErrInternal("更新项目成员失败")
			}
			updated, err := s.repo.Project.GetByID(ctx, id)
			if err != nil {
				return nil, ErrInternal("获取项目信息失败")
			}
			return updated, nil
		}
		existing := map[int]string{}
		for _, member := range existingMembers {
			existing[member.UserID] = member.Role
		}
		next := map[int]string{}
		for _, member := range *members {
			next[member.UserID] = member.Role
		}
		for _, member := range existingMembers {
			if _, ok := next[member.UserID]; !ok {
				return nil, ErrForbidden("当前角色不能删除项目成员")
			}
		}
		additions := make([]models.ProjectMember, 0)
		selfRoleChanged := false
		for _, member := range *members {
			if role, ok := existing[member.UserID]; ok {
				if role != member.Role {
					if member.UserID == userID && canSelfUpdateProjectRole(userID, member.Role, existingMembers) {
						selfRoleChanged = true
						continue
					}
					return nil, ErrForbidden("当前角色不能修改已有成员角色")
				}
				continue
			}
			additions = append(additions, member)
		}
		if selfRoleChanged {
			if err := s.repo.Project.ReplaceMembers(ctx, id, *members); err != nil {
				log.Printf("[ProjectService.UpdateProject] repository error replacing members for self role update: %v", err)
				return nil, ErrInternal("更新项目成员失败")
			}
		} else if err := s.repo.Project.AddMembers(ctx, id, additions); err != nil {
			log.Printf("[ProjectService.UpdateProject] repository error adding members: %v", err)
			return nil, ErrInternal("添加项目成员失败")
		}
		updated, err := s.repo.Project.GetByID(ctx, id)
		if err != nil {
			return nil, ErrInternal("获取项目信息失败")
		}
		return updated, nil
	}

	contentUpdate := input.Name != nil || input.Description != nil || input.Direction != nil ||
		input.MemberCount != nil || input.IsCrossSchool != nil || input.EducationRequirement != nil ||
		input.SkillRequirement != nil || input.Tags != nil || input.PublisherRole != nil ||
		input.SchoolID != nil || input.InitiatingSchoolID != nil || input.NeedReview != nil
	if contentUpdate && !isOwner {
		return nil, ErrForbidden("只有项目创建者可以修改项目内容")
	}

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
	if err := projectRepo.UpdateWithMetadata(ctx, project, input.Tags, input.PublisherRole, input.InitiatingSchoolID, milestones, members, input.EventIDs); err != nil {
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

func (s *ProjectService) assertProjectOperator(ctx context.Context, id, userID int, action string) (*models.Project, error) {
	project, err := s.repo.Project.GetByID(ctx, id)
	if err != nil {
		log.Printf("[ProjectService.%s] repository error getting project: %v", action, err)
		return nil, ErrInternal("获取项目信息失败")
	}
	if project == nil {
		return nil, ErrNotFound("项目不存在")
	}
	members, err := s.repo.Project.ListMembers(ctx, id)
	if err != nil {
		log.Printf("[ProjectService.%s] repository error listing members: %v", action, err)
		return nil, ErrInternal("检查权限失败")
	}
	currentRole, _ := currentUserProjectRole(project, userID, members)
	if !canOperateAsHighestRole(currentRole, members) {
		return nil, ErrForbidden("当前角色无权操作项目")
	}
	return project, nil
}

func requireProjectStatus(project *models.Project, allowed map[int]struct{}, action string) error {
	if project == nil {
		return ErrNotFound("项目不存在")
	}
	if _, ok := allowed[project.Status]; ok {
		return nil
	}
	return ErrBadRequest(action + "当前项目状态不允许操作")
}

// DeleteProject checks ownership and soft-deletes the project.
func (s *ProjectService) DeleteProject(ctx context.Context, id, userID int) error {
	project, err := s.assertProjectOperator(ctx, id, userID, "DeleteProject")
	if err != nil {
		return err
	}
	if err := requireProjectStatus(project, map[int]struct{}{
		models.ProjectStatusPending:          {},
		models.ProjectStatusApproved:         {},
		models.ProjectStatusRejected:         {},
		models.ProjectStatusRecruitCompleted: {},
	}, "删除项目"); err != nil {
		return err
	}

	if err := s.repo.Project.Delete(ctx, id); err != nil {
		log.Printf("[ProjectService.DeleteProject] repository error: %v", err)
		return ErrInternal("删除项目失败")
	}

	return nil
}

func (s *ProjectService) RestoreProjectByUser(ctx context.Context, id, userID int) error {
	project, err := s.assertProjectOperator(ctx, id, userID, "RestoreProjectByUser")
	if err != nil {
		return err
	}
	return s.restoreProject(ctx, id, project)
}

func (s *ProjectService) CompleteRecruit(ctx context.Context, id, userID int) error {
	project, err := s.assertProjectOperator(ctx, id, userID, "CompleteRecruit")
	if err != nil {
		return err
	}
	if err := requireProjectStatus(project, map[int]struct{}{
		models.ProjectStatusApproved: {},
	}, "完成招募"); err != nil {
		return err
	}
	if _, err := s.repo.Project.CompleteRecruit(ctx, id); err != nil {
		log.Printf("[ProjectService.CompleteRecruit] repository error: %v", err)
		return ErrInternal("完成招募失败")
	}
	return nil
}

func (s *ProjectService) RestartRecruit(ctx context.Context, id, userID int) error {
	project, err := s.assertProjectOperator(ctx, id, userID, "RestartRecruit")
	if err != nil {
		return err
	}
	if err := requireProjectStatus(project, map[int]struct{}{
		models.ProjectStatusRecruitCompleted: {},
	}, "重启招募"); err != nil {
		return err
	}
	if err := s.repo.Project.UpdateStatus(ctx, id, models.ProjectStatusPending); err != nil {
		log.Printf("[ProjectService.RestartRecruit] repository error: %v", err)
		return ErrInternal("重启招募失败")
	}
	return nil
}

func (s *ProjectService) EndProject(ctx context.Context, id, userID int) error {
	project, err := s.assertProjectOperator(ctx, id, userID, "EndProject")
	if err != nil {
		return err
	}
	if err := requireProjectStatus(project, map[int]struct{}{
		models.ProjectStatusRecruitCompleted: {},
	}, "结束项目"); err != nil {
		return err
	}
	if err := s.repo.Project.UpdateStatus(ctx, id, models.ProjectStatusEnded); err != nil {
		log.Printf("[ProjectService.EndProject] repository error: %v", err)
		return ErrInternal("结束项目失败")
	}
	return nil
}

func (s *ProjectService) RestoreProject(ctx context.Context, id int) error {
	project, err := s.repo.Project.GetByID(ctx, id)
	if err != nil {
		log.Printf("[ProjectService.RestoreProject] repository error getting project: %v", err)
		return ErrInternal("获取项目信息失败")
	}
	if project == nil {
		return ErrNotFound("项目不存在")
	}
	return s.restoreProject(ctx, id, project)
}

func (s *ProjectService) restoreProject(ctx context.Context, id int, project *models.Project) error {
	if err := requireProjectStatus(project, map[int]struct{}{
		models.ProjectStatusDeleting: {},
	}, "恢复项目"); err != nil {
		return err
	}
	if err := s.repo.Project.UpdateStatus(ctx, id, models.ProjectStatusPending); err != nil {
		log.Printf("[ProjectService.RestoreProject] repository error: %v", err)
		return ErrInternal("恢复项目失败")
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

// ListProjectApplications returns paginated applications for a project reviewer.
func (s *ProjectService) ListProjectApplications(ctx context.Context, projectID, userID int, params repository.ApplicationListParams) (*ApplicationListResult, error) {
	params.Page, params.Size = normalizePageParams(params.Page, params.Size)

	project, err := s.repo.Project.GetByID(ctx, projectID)
	if err != nil {
		log.Printf("[ProjectService.ListProjectApplications] repository error getting project: %v", err)
		return nil, ErrInternal("检查权限失败")
	}
	if project == nil {
		return nil, ErrNotFound("项目不存在")
	}
	members, err := s.repo.Project.ListMembers(ctx, projectID)
	if err != nil {
		log.Printf("[ProjectService.ListProjectApplications] repository error listing members: %v", err)
		return nil, ErrInternal("检查权限失败")
	}
	currentRole, _ := currentUserProjectRole(project, userID, members)
	if currentRole == nil {
		return nil, ErrForbidden("无权查看申请列表")
	}

	params.ProjectID = &projectID

	applications, total, err := s.repo.Application.List(ctx, params)
	if err != nil {
		log.Printf("[ProjectService.ListProjectApplications] repository error: %v", err)
		return nil, ErrInternal("获取申请列表失败")
	}
	for i := range applications {
		canReview := canReviewApplicationByRole(userID, currentRole, &applications[i])
		applications[i].CanReview = &canReview
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
	if status != models.ApplicationStatusDiscussing && status != models.ApplicationStatusRejected {
		return ErrBadRequest("不支持的申请操作")
	}

	app, err := s.repo.Application.GetByID(ctx, applicationID)
	if err != nil {
		log.Printf("[ProjectService.ReviewApplication] repository error getting application: %v", err)
		return ErrInternal("获取申请信息失败")
	}
	if app == nil {
		return ErrNotFound("申请不存在")
	}
	if app.Status == models.ApplicationStatusRejected || app.Status == models.ApplicationStatusJoined {
		return ErrBadRequest("当前申请状态不允许操作")
	}
	if status == models.ApplicationStatusDiscussing && app.Status != models.ApplicationStatusPending {
		return ErrBadRequest("只有待审核申请可以进入互相了解")
	}

	project, err := s.repo.Project.GetByID(ctx, app.ProjectID)
	if err != nil {
		log.Printf("[ProjectService.ReviewApplication] repository error getting project: %v", err)
		return ErrInternal("获取项目信息失败")
	}
	if project == nil {
		return ErrNotFound("项目不存在")
	}
	members, err := s.repo.Project.ListMembers(ctx, app.ProjectID)
	if err != nil {
		log.Printf("[ProjectService.ReviewApplication] repository error listing members: %v", err)
		return ErrInternal("检查权限失败")
	}
	reviewerRole, _ := currentUserProjectRole(project, userID, members)
	if reviewerRole == nil {
		return ErrForbidden("无权审核申请")
	}
	if app.Status != models.ApplicationStatusPending && !canReviewApplicationByRole(userID, reviewerRole, app) {
		return ErrForbidden("当前角色无权操作该申请")
	}

	if err := s.repo.Application.UpdateStatusWithReviewer(ctx, applicationID, int(status), userID, reviewerRole); err != nil {
		log.Printf("[ProjectService.ReviewApplication] repository error updating status: %v", err)
		return ErrInternal("更新申请状态失败")
	}

	go func(asyncCtx context.Context) {
		resultStr := "正在互相了解"
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

		err = s.message.SendSubscribeMsgByBizKey(asyncCtx, app.UserID, models.MsgBizKeyCardDeliveryResult, data)
		if err != nil {
			log.Printf("[ProjectService.ReviewApplication] notification error: %v", err)
		}
	}(context.WithoutCancel(ctx))

	return nil
}

type AssignApplicationRoleInput struct {
	ApplicationID int
	UserID        int
	Role          string
}

func (s *ProjectService) AssignApplicationRole(ctx context.Context, input AssignApplicationRoleInput) error {
	role := strings.TrimSpace(input.Role)
	if input.ApplicationID <= 0 || input.UserID <= 0 || role == "" {
		return ErrBadRequest("参数错误")
	}

	app, err := s.repo.Application.GetByID(ctx, input.ApplicationID)
	if err != nil {
		log.Printf("[ProjectService.AssignApplicationRole] repository error getting application: %v", err)
		return ErrInternal("获取申请信息失败")
	}
	if app == nil {
		return ErrNotFound("申请不存在")
	}
	if app.Status != models.ApplicationStatusDiscussing {
		return ErrBadRequest("只有正在互相了解的申请可以同意入队")
	}
	project, err := s.repo.Project.GetByID(ctx, app.ProjectID)
	if err != nil {
		log.Printf("[ProjectService.AssignApplicationRole] repository error getting project: %v", err)
		return ErrInternal("获取项目信息失败")
	}
	if project == nil {
		return ErrNotFound("项目不存在")
	}
	members, err := s.repo.Project.ListMembers(ctx, app.ProjectID)
	if err != nil {
		log.Printf("[ProjectService.AssignApplicationRole] repository error listing members: %v", err)
		return ErrInternal("检查权限失败")
	}
	currentRole, _ := currentUserProjectRole(project, input.UserID, members)
	if !canReviewApplicationByRole(input.UserID, currentRole, app) {
		return ErrForbidden("当前角色无权操作该申请")
	}
	if !canAssignProjectRole(currentRole, role) {
		return ErrForbidden("当前角色不能分配该团队角色")
	}
	exists, err := s.repo.Project.RoleExists(ctx, role)
	if err != nil {
		return ErrInternal("校验项目角色失败")
	}
	if !exists {
		return ErrBadRequest("项目角色不存在")
	}

	tx, err := s.repo.DB().BeginTxx(ctx, nil)
	if err != nil {
		return ErrInternal("开启事务失败")
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO project_members(project_id,user_id,role)
		VALUES(?,?,?)
		ON DUPLICATE KEY UPDATE role=VALUES(role), updated_at=CURRENT_TIMESTAMP`, app.ProjectID, app.UserID, role); err != nil {
		log.Printf("[ProjectService.AssignApplicationRole] add member failed: %v", err)
		return ErrInternal("添加项目成员失败")
	}
	result, err := tx.ExecContext(ctx, `UPDATE project_application
		SET status=?, assigned_role=?, joined_at=CURRENT_TIMESTAMP, updated_at=CURRENT_TIMESTAMP
		WHERE id=?`, models.ApplicationStatusJoined, role, input.ApplicationID)
	if err != nil {
		log.Printf("[ProjectService.AssignApplicationRole] update application failed: %v", err)
		return ErrInternal("更新申请状态失败")
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound("申请不存在")
	}
	if err := tx.Commit(); err != nil {
		return ErrInternal("提交事务失败")
	}
	return nil
}

// TakedownProject (admin only) sets an approved project to closed/taken-down.
func (s *ProjectService) TakedownProject(ctx context.Context, id int, rejectReason *string) error {
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

	if err := s.repo.Project.UpdateStatusWithRejectReason(ctx, id, models.ProjectStatusRejected, rejectReason); err != nil {
		log.Printf("[ProjectService.TakedownProject] repository error updating status: %v", err)
		return ErrInternal("下架失败")
	}

	go func(asyncCtx context.Context) {
		remark := "请按照审核意见重新提交项目。"
		if rejectReason != nil && strings.TrimSpace(*rejectReason) != "" {
			remark = truncate20WithEllipsis(strings.TrimSpace(*rejectReason))
		}
		data := map[string]string{
			"project_name": truncate20(project.Name),
			"status":       "审核拒绝",
			"remark":       remark,
		}
		if err := s.message.SendSubscribeMsgByBizKey(asyncCtx, project.CreatorID, models.MsgBizKeyAuditResultProj, data); err != nil {
			log.Printf("[ProjectService.TakedownProject] notification error: %v", err)
		}
	}(context.WithoutCancel(ctx))

	return nil
}

// ReviewProject (admin only) updates project status and notifies creator.
func (s *ProjectService) ReviewProject(ctx context.Context, id, status int, rejectReason *string) error {
	project, err := s.repo.Project.GetByID(ctx, id)
	if err != nil {
		log.Printf("[ProjectService.ReviewProject] repository error: %v", err)
		return ErrInternal("获取项目失败")
	}
	if project == nil {
		return ErrNotFound("项目不存在")
	}

	if status == models.ProjectStatusRejected {
		if err := s.repo.Project.UpdateStatusWithRejectReason(ctx, id, status, rejectReason); err != nil {
			log.Printf("[ProjectService.ReviewProject] repository error updating status: %v", err)
			return ErrInternal("审核失败")
		}
	} else if err := s.repo.Project.UpdateStatus(ctx, id, status); err != nil {
		log.Printf("[ProjectService.ReviewProject] repository error updating status: %v", err)
		return ErrInternal("审核失败")
	}
	if project.Status == models.ProjectStatusPending &&
		(status == models.ProjectStatusApproved || status == models.ProjectStatusRejected) {
		if err := s.repo.Project.MarkPassiveStatusChange(ctx, id); err != nil {
			log.Printf("[ProjectService.ReviewProject] repository error marking passive status change: %v", err)
			return ErrInternal("审核失败")
		}
	}

	// 向项目负责人发送审核结果通知
	go func(asyncCtx context.Context) {
		statusStr := "审核通过"
		remark := "项目已上线，快去查看吧！" // 12 字 ≤ thing7 上限 20 字
		if status == models.ProjectStatusRejected {
			statusStr = "审核拒绝"
			if rejectReason != nil && strings.TrimSpace(*rejectReason) != "" {
				remark = truncate20WithEllipsis(strings.TrimSpace(*rejectReason))
			} else {
				remark = "请按照审核意见重新提交项目。" // 14 字 ≤ thing7 上限 20 字
			}
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
