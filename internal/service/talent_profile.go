package service

import (
	"context"
	"log"

	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

// TalentProfileService handles talent profile business logic.
type TalentProfileService struct {
	repo         *repository.Repository
	contentAudit *ContentAuditService
}

// NewTalentProfileService creates a new TalentProfileService.
func NewTalentProfileService(repo *repository.Repository, contentAudit *ContentAuditService) *TalentProfileService {
	return &TalentProfileService{repo: repo, contentAudit: contentAudit}
}

// resolveUpsertStatus determines the actual status to save based on the requested status for upsert operations.
func (s *TalentProfileService) resolveUpsertStatus(status *api.TalentStatus) (*int, error) {
	if status == nil {
		resolved := models.TalentStatusPrivate
		return &resolved, nil
	}

	statusInt := int(*status)
	if err := IsValidStatus("talent_profile.status", statusInt); err != nil {
		return nil, err
	}

	if statusInt == models.TalentStatusOnline {
		resolved := models.TalentStatusReviewing
		return &resolved, nil
	}

	return &statusInt, nil
}

// UpsertTalentProfile creates or updates the current user's talent profile.
func (s *TalentProfileService) UpsertTalentProfile(ctx context.Context, userID int, req api.UpsertTalentProfileDTO) (*models.TalentProfile, error) {
	status, err := s.resolveUpsertStatus(req.Status)
	if err != nil {
		return nil, err
	}

	var auditTexts []string
	if req.SelfEvaluation != nil {
		auditTexts = append(auditTexts, *req.SelfEvaluation)
	}
	if req.ProjectExperience != nil {
		auditTexts = append(auditTexts, *req.ProjectExperience)
	}
	if len(auditTexts) > 0 {
		if err := s.contentAudit.CheckText(ctx, auditTexts...); err != nil {
			return nil, err
		}
	}

	var skillSummary models.JSONStringArray
	if req.Skills != nil {
		skillSummary = models.JSONStringArray{
			Items: append([]string(nil), (*req.Skills)...),
			Valid: true,
		}
	}

	profile := &models.TalentProfile{
		UserID:            userID,
		SelfEvaluation:    req.SelfEvaluation,
		SkillSummary:      skillSummary,
		ProjectExperience: req.ProjectExperience,
		MBTI:              req.Mbti,
		Status:            status,
	}

	if err := s.repo.TalentProfile.Upsert(ctx, profile); err != nil {
		log.Printf("[TalentProfileService.UpsertTalentProfile] repository error: %v", err)
		return nil, ErrInternal("保存人才档案失败")
	}

	updated, err := s.repo.TalentProfile.GetByUserID(ctx, userID)
	if err != nil {
		log.Printf("[TalentProfileService.UpsertTalentProfile] repository error reloading: %v", err)
		return nil, ErrInternal("获取人才档案失败")
	}
	if updated == nil {
		return nil, ErrNotFound("人才档案不存在")
	}

	return updated, nil
}

// SetTalentProfilePrivate hides the current user's talent profile without deleting it.
func (s *TalentProfileService) SetTalentProfilePrivate(ctx context.Context, userID int) error {
	profile, err := s.repo.TalentProfile.GetByUserID(ctx, userID)
	if err != nil {
		log.Printf("[TalentProfileService.SetTalentProfilePrivate] repository error getting profile: %v", err)
		return ErrInternal("获取人才档案失败")
	}
	if profile == nil {
		return ErrNotFound("人才档案不存在")
	}

	status := models.TalentStatusPrivate
	profile.Status = &status
	if err := s.repo.TalentProfile.Upsert(ctx, profile); err != nil {
		log.Printf("[TalentProfileService.SetTalentProfilePrivate] repository error updating status: %v", err)
		return ErrInternal("下架人才档案失败")
	}

	return nil
}

// AdminListTalentProfiles returns a paginated list of talent profiles for admin.
func (s *TalentProfileService) AdminListTalentProfiles(ctx context.Context, params repository.TalentProfileAdminListParams) ([]models.TalentProfile, int64, error) {
	profiles, total, err := s.repo.TalentProfile.ListAdmin(ctx, params)
	if err != nil {
		log.Printf("[TalentProfileService.AdminListTalentProfiles] repository error: %v", err)
		return nil, 0, ErrInternal("获取名片列表失败")
	}
	return profiles, total, nil
}

// AdminGetTalentProfile returns the full detail of a talent profile for admin.
func (s *TalentProfileService) AdminGetTalentProfile(ctx context.Context, id int) (*models.TalentProfile, error) {
	profile, err := s.repo.TalentProfile.GetByIDForAdmin(ctx, id)
	if err != nil {
		log.Printf("[TalentProfileService.AdminGetTalentProfile] repository error: %v", err)
		return nil, ErrInternal("获取名片详情失败")
	}
	if profile == nil {
		return nil, ErrNotFound("名片不存在")
	}
	return profile, nil
}

// TakedownTalentProfile forcibly takes down an online talent profile (status 1 → 0).
func (s *TalentProfileService) TakedownTalentProfile(ctx context.Context, id int) error {
	profile, err := s.repo.TalentProfile.GetByID(ctx, id)
	if err != nil {
		log.Printf("[TalentProfileService.TakedownTalentProfile] repository error getting profile: %v", err)
		return ErrInternal("获取名片失败")
	}
	if profile == nil {
		return ErrNotFound("名片不存在")
	}
	if profile.Status == nil || *profile.Status != models.TalentStatusOnline {
		return ErrBadRequest("当前名片未上架，无法执行下架操作")
	}

	if err := s.repo.TalentProfile.UpdateStatus(ctx, id, models.TalentStatusTakenDown); err != nil {
		log.Printf("[TalentProfileService.TakedownTalentProfile] repository error updating status: %v", err)
		return ErrInternal("下架失败")
	}
	return nil
}

// ReviewTalentProfile reviews a talent profile from reviewing to approved or private.
func (s *TalentProfileService) ReviewTalentProfile(ctx context.Context, id, status int) error {
	if status != models.TalentStatusPrivate && status != models.TalentStatusOnline {
		return ErrBadRequest("无效的人才档案状态")
	}

	profile, err := s.repo.TalentProfile.GetByID(ctx, id)
	if err != nil {
		log.Printf("[TalentProfileService.ReviewTalentProfile] repository error getting profile: %v", err)
		return ErrInternal("获取人才档案失败")
	}
	if profile == nil {
		return ErrNotFound("人才档案不存在")
	}
	if profile.Status == nil || *profile.Status != models.TalentStatusReviewing {
		return ErrBadRequest("当前人才档案状态不允许审核")
	}

	if err := s.repo.TalentProfile.UpdateStatus(ctx, id, status); err != nil {
		log.Printf("[TalentProfileService.ReviewTalentProfile] repository error updating status: %v", err)
		return ErrInternal("审核失败")
	}

	return nil
}
