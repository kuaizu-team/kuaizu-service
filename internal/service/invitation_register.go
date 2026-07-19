package service

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/messagecenter"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

var mainlandPhonePattern = regexp.MustCompile(`^1[3-9]\d{9}$`)

type registerInviteSmsSender interface {
	SendRegisterInviteSms(ctx context.Context, req messagecenter.RegisterInviteSmsRequest) (*messagecenter.RegisterInviteSmsResponse, error)
}

type RegisterInvitationService struct {
	repo                 *repository.Repository
	mu                   sync.RWMutex
	messageCenter        registerInviteSmsSender
	messageCenterInitErr error
	messageCenterFactory func() (*messagecenter.Client, error)
}

func NewRegisterInvitationService(repo *repository.Repository, messageCenter *messagecenter.Client, messageCenterInitErr error) *RegisterInvitationService {
	svc := &RegisterInvitationService{
		repo:                 repo,
		messageCenterInitErr: messageCenterInitErr,
		messageCenterFactory: messagecenter.NewClientFromEnv,
	}
	if messageCenter != nil {
		svc.messageCenter = messageCenter
	}
	return svc
}

type PhoneCheckResult struct {
	Exists  bool         `json:"exists"`
	Invited bool         `json:"invited"`
	User    *models.User `json:"user,omitempty"`
}

type RegisterInviteInput struct {
	Phone     string
	ProjectID int
	Role      string
}

func (s *RegisterInvitationService) CheckByPhone(ctx context.Context, phone string, projectID int) (*PhoneCheckResult, error) {
	phone = strings.TrimSpace(phone)
	if !mainlandPhonePattern.MatchString(phone) {
		return nil, ErrBadRequest("请输入正确手机号")
	}
	user, err := s.repo.User.GetByPhone(ctx, phone)
	if err != nil {
		log.Printf("[RegisterInvitationService.CheckByPhone] user lookup error: %v", err)
		return nil, ErrInternal("检查手机号失败")
	}
	result := &PhoneCheckResult{Exists: user != nil, User: user}
	if projectID > 0 {
		record, err := s.repo.InvitationRecord.GetByPhoneProject(ctx, phone, projectID)
		if err != nil {
			log.Printf("[RegisterInvitationService.CheckByPhone] invitation lookup error: %v", err)
			return nil, ErrInternal("检查邀请状态失败")
		}
		result.Invited = record != nil && record.Status == models.InvitationRecordStatusSent
	}
	return result, nil
}

func (s *RegisterInvitationService) Invite(ctx context.Context, inviterUserID int, input RegisterInviteInput) (*models.InvitationRecord, error) {
	phone := strings.TrimSpace(input.Phone)
	if !mainlandPhonePattern.MatchString(phone) {
		return nil, ErrBadRequest("请输入正确手机号")
	}
	if input.ProjectID <= 0 {
		return nil, ErrBadRequest("项目ID不能为空")
	}
	role := strings.TrimSpace(input.Role)
	if role == "" {
		role = models.ProjectRoleTeamMember
	}
	if ok, err := s.repo.Project.RoleExists(ctx, role); err != nil {
		return nil, ErrInternal("检查项目角色失败")
	} else if !ok {
		return nil, ErrBadRequest("项目角色不存在")
	}

	existingUser, err := s.repo.User.GetByPhone(ctx, phone)
	if err != nil {
		return nil, ErrInternal("检查手机号失败")
	}
	if existingUser != nil {
		return nil, ErrBadRequest("该手机号已注册，请直接添加成员")
	}

	project, err := s.repo.Project.GetByID(ctx, input.ProjectID)
	if err != nil {
		return nil, ErrInternal("获取项目失败")
	}
	if project == nil {
		return nil, ErrNotFound("项目不存在")
	}
	members, err := s.repo.Project.ListMembers(ctx, project.ID)
	if err != nil {
		return nil, ErrInternal("获取项目成员失败")
	}
	currentRole, _ := currentUserProjectRole(project, inviterUserID, members)
	if !canOperateAsHighestRole(currentRole, members) {
		return nil, ErrForbidden("无权邀请项目成员")
	}

	existing, err := s.repo.InvitationRecord.GetByPhoneProject(ctx, phone, project.ID)
	if err != nil {
		return nil, ErrInternal("检查邀请记录失败")
	}
	if existing != nil && existing.Status == models.InvitationRecordStatusSent {
		return existing, nil
	}

	now := time.Now()
	record := &models.InvitationRecord{
		Phone:         phone,
		ProjectID:     project.ID,
		InviterUserID: inviterUserID,
		Role:          role,
		Status:        models.InvitationRecordStatusFailed,
		SentAt:        &now,
	}
	if existing != nil {
		record = existing
		record.InviterUserID = inviterUserID
		record.Role = role
		record.Status = models.InvitationRecordStatusFailed
		record.ErrorMessage = nil
		record.SentAt = &now
		if err := s.repo.InvitationRecord.Update(ctx, record); err != nil {
			return nil, ErrInternal("更新邀请记录失败")
		}
	} else if err := s.repo.InvitationRecord.Create(ctx, record); err != nil {
		return nil, ErrInternal("创建邀请记录失败")
	}

	resp, err := s.sendSms(ctx, record, project, role)
	if err != nil {
		msg := err.Error()
		record.Status = models.InvitationRecordStatusFailed
		record.ErrorMessage = &msg
		_ = s.repo.InvitationRecord.Update(context.WithoutCancel(ctx), record)
		log.Printf("[RegisterInvitationService.Invite] send sms failed record_id=%d project_id=%d: %v", record.ID, project.ID, err)
		return nil, ErrInternal("发送邀请短信失败")
	}

	record.Status = models.InvitationRecordStatusSent
	record.ErrorMessage = nil
	if resp != nil {
		if resp.Provider != "" {
			record.Provider = &resp.Provider
		}
		providerBizID := resp.ProviderBizID
		if providerBizID == "" {
			providerBizID = resp.TaskID
		}
		if providerBizID != "" {
			record.ProviderBizID = &providerBizID
		}
	}
	if err := s.repo.InvitationRecord.Update(ctx, record); err != nil {
		return nil, ErrInternal("更新邀请记录失败")
	}
	return record, nil
}

func (s *RegisterInvitationService) sendSms(ctx context.Context, record *models.InvitationRecord, project *models.Project, role string) (*messagecenter.RegisterInviteSmsResponse, error) {
	submitter, initErr, baseURL := s.resolveMessageCenter()
	if initErr != nil {
		return nil, fmt.Errorf("message center unavailable base_url_empty=%t: %w", baseURL == "", initErr)
	}
	if submitter == nil {
		return nil, fmt.Errorf("message center client is nil")
	}
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return submitter.SendRegisterInviteSms(callCtx, messagecenter.RegisterInviteSmsRequest{
		TraceID:      fmt.Sprintf("REGISTER_INVITE:%d", record.ID),
		RecordID:     record.ID,
		Phone:        record.Phone,
		ProjectID:    project.ID,
		ProjectTitle: truncate20(project.Name),
		TeamRole:     truncate20(projectRoleLabel(role)),
	})
}

func (s *RegisterInvitationService) resolveMessageCenter() (registerInviteSmsSender, error, string) {
	s.mu.RLock()
	submitter := s.messageCenter
	initErr := s.messageCenterInitErr
	baseURL := registerInviteMessageCenterBaseURL(submitter)
	s.mu.RUnlock()
	if initErr == nil || submitter != nil {
		return submitter, initErr, baseURL
	}

	factory := s.messageCenterFactory
	if factory == nil {
		factory = messagecenter.NewClientFromEnv
	}
	client, err := factory()
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.messageCenterInitErr = err
		return s.messageCenter, s.messageCenterInitErr, registerInviteMessageCenterBaseURL(s.messageCenter)
	}
	s.messageCenter = client
	s.messageCenterInitErr = nil
	return s.messageCenter, nil, client.BaseURL()
}

func registerInviteMessageCenterBaseURL(submitter registerInviteSmsSender) string {
	if client, ok := submitter.(*messagecenter.Client); ok {
		return client.BaseURL()
	}
	return ""
}

func projectRoleLabel(role string) string {
	switch role {
	case models.ProjectRoleTeamLeader:
		return "团队负责人"
	case models.ProjectRoleLearningMember:
		return "学习成员"
	case "RECRUITMENT_LEADER":
		return "招募负责人"
	case "TECH_LEADER":
		return "技术负责人"
	case "OPERATIONS_LEADER":
		return "运营负责人"
	case "PUBLICITY_LEADER":
		return "宣传负责人"
	case "DESIGN_LEADER":
		return "美化负责人"
	case "LEGAL_LEADER":
		return "法务负责人"
	default:
		return "团队成员"
	}
}
