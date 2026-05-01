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

// resolveUpsertStatus determines the actual status to save based on the requested status and the current status.
//
// Rules:
//   - User does not specify a status (nil):
//     · If the profile is currently Online (1) → move to Reviewing (2) so admin re-approves after edit
//     · Otherwise keep the existing status; default to Private (0) for brand-new profiles
//   - User explicitly requests Online (1) → demote to Reviewing (2) to prevent self-approval
//   - Any other explicit status → use as-is (after validation)
func (s *TalentProfileService) resolveUpsertStatus(requestedStatus *api.TalentStatus, currentStatus *int) (*int, error) {
	if requestedStatus == nil {
		// Pure content edit — apply automatic status transition.
		if currentStatus != nil && *currentStatus == models.TalentStatusOnline {
			// 已上架 → 编辑后进入待审核，管理员重新审核后再上架
			resolved := models.TalentStatusReviewing
			return &resolved, nil
		}
		// 待审核或已下架 → 保持原状态不变；全新档案 → 默认下架
		if currentStatus != nil {
			return currentStatus, nil
		}
		resolved := models.TalentStatusPrivate
		return &resolved, nil
	}

	statusInt := int(*requestedStatus)
	if err := IsValidStatus("talent_profile.status", statusInt); err != nil {
		return nil, err
	}

	// 用户不能直接将自己的状态设为"已上架"，必须经过管理员审核
	if statusInt == models.TalentStatusOnline {
		resolved := models.TalentStatusReviewing
		return &resolved, nil
	}

	// 前端有时会在编辑内容时显式传 status=0（如回传当前值或默认值），
	// 若当前名片已上架，不应因此直接下架——同样转为待审核。
	// 注意：用户主动下架有专用的 DELETE /talent-profiles/my 接口，
	// 因此在 Upsert 路径里拦截此情况是安全的。
	if statusInt == models.TalentStatusPrivate &&
		currentStatus != nil && *currentStatus == models.TalentStatusOnline {
		resolved := models.TalentStatusReviewing
		return &resolved, nil
	}

	return &statusInt, nil
}

// UpsertTalentProfile creates or updates the current user's talent profile.
func (s *TalentProfileService) UpsertTalentProfile(ctx context.Context, userID int, req api.UpsertTalentProfileDTO) (*models.TalentProfile, error) {
	// 先查询现有档案，以便 resolveUpsertStatus 根据当前状态做正确的状态转换
	existing, err := s.repo.TalentProfile.GetByUserID(ctx, userID)
	if err != nil {
		log.Printf("[TalentProfileService.UpsertTalentProfile] repository error getting existing profile: %v", err)
		return nil, ErrInternal("获取人才档案失败")
	}
	var currentStatus *int
	if existing != nil {
		currentStatus = existing.Status
	}

	status, err := s.resolveUpsertStatus(req.Status, currentStatus)
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

// TakedownTalentProfile (admin only) forces an online profile offline (status: 1 → 0).
func (s *TalentProfileService) TakedownTalentProfile(ctx context.Context, id int) error {
	profile, err := s.repo.TalentProfile.GetByID(ctx, id)
	if err != nil {
		log.Printf("[TalentProfileService.TakedownTalentProfile] repository error getting profile: %v", err)
		return ErrInternal("获取人才档案失败")
	}
	if profile == nil {
		return ErrNotFound("人才档案不存在")
	}
	if profile.Status == nil || *profile.Status != models.TalentStatusOnline {
		return ErrBadRequest("当前名片状态不是已上架，无法下架")
	}

	if err := s.repo.TalentProfile.UpdateStatus(ctx, id, models.TalentStatusPrivate); err != nil {
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
