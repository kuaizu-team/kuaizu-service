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
	message      *MessageService
}

// NewTalentProfileService creates a new TalentProfileService.
func NewTalentProfileService(repo *repository.Repository, contentAudit *ContentAuditService, message *MessageService) *TalentProfileService {
	return &TalentProfileService{repo: repo, contentAudit: contentAudit, message: message}
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

	// 前端有时会在编辑内容时显式传 status=0（如回传当前值或默认值）。
	// Upsert 接口仅用于编辑内容，用户主动下架有专用的 DELETE /talent-profiles/my 接口，
	// 因此在此路径拦截"误传 0"是安全的：
	//   · 当前已上架(1) + 传 0 → 转为待审核(2)，走正常审核流程
	//   · 当前待审核(2) + 传 0 → 保持待审核(2)不变，不应因编辑而中断审核
	if statusInt == models.TalentStatusPrivate && currentStatus != nil {
		if *currentStatus == models.TalentStatusOnline {
			resolved := models.TalentStatusReviewing
			return &resolved, nil
		}
		if *currentStatus == models.TalentStatusReviewing {
			return currentStatus, nil
		}
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

// GetTalentProfile returns a talent profile without recording a view.
func (s *TalentProfileService) GetTalentProfile(ctx context.Context, id int) (*models.TalentProfile, error) {
	profile, err := s.repo.TalentProfile.GetByID(ctx, id)
	if err != nil {
		log.Printf("[TalentProfileService.GetTalentProfile] repository error: %v", err)
		return nil, ErrInternal("获取人才档案失败")
	}
	if profile == nil {
		return nil, ErrNotFound("人才档案不存在")
	}
	return profile, nil
}

// GetTalentProfileWithView returns a talent profile and asynchronously records a real view.
func (s *TalentProfileService) GetTalentProfileWithView(ctx context.Context, id, viewerUserID, source int) (*models.TalentProfile, error) {
	profile, err := s.GetTalentProfile(ctx, id)
	if err != nil {
		return nil, err
	}

	go func(asyncCtx context.Context) {
		if err := s.repo.TalentProfile.IncrementViewCount(asyncCtx, id); err != nil {
			log.Printf("[TalentProfileService.GetTalentProfileWithView] increment view error (non-fatal): %v", err)
		}
		var uidPtr *int
		if viewerUserID > 0 {
			uid := viewerUserID
			uidPtr = &uid
		}
		entry := &models.TalentViewLog{
			TalentID: id,
			UserID:   uidPtr,
			Source:   source,
		}
		if err := s.repo.TalentViewLog.InsertViewLog(asyncCtx, entry); err != nil {
			log.Printf("[TalentProfileService.GetTalentProfileWithView] view log error (non-fatal): %v", err)
			return
		}
		if viewerUserID <= 0 || viewerUserID == profile.UserID {
			return
		}
		if s.message == nil {
			return
		}
		viewer, err := s.repo.User.GetByID(asyncCtx, viewerUserID)
		if err != nil {
			log.Printf("[TalentProfileService.GetTalentProfileWithView] get viewer error (non-fatal): %v", err)
		}
		notification, ok := buildTalentVisitNotification(viewerUserID, profile, notificationUserName(viewer))
		if ok {
			sendSubscribeNotification(asyncCtx, s.message, notification)
		}
	}(context.WithoutCancel(ctx))

	return profile, nil
}

// TalentDashboardResult is the response payload for GET /talent-profiles/{id}/dashboard.
type TalentDashboardResult struct {
	TotalViews                int                          `json:"total_views"`
	TodayViews                int                          `json:"today_views"`
	AvgDurationSeconds        int                          `json:"avg_duration_seconds"`
	HourlyViews               []repository.HourlyViewItem  `json:"hourly_views"`
	LikeCount                 int                          `json:"like_count"`
	FavoriteCount             int                          `json:"favorite_count"`
	ShareCount                int                          `json:"share_count"`
	VisitCount                int                          `json:"visit_count"`
	InteractionUnread         repository.InteractionUnread `json:"interaction_unread"`
	VisitUnreadCount          int                          `json:"visit_unread_count"`
	AppliedProjectTotal       int                          `json:"applied_project_total"`
	ApplicationReadRate       float64                      `json:"application_read_rate"`
	ApplicationAgreeRate      float64                      `json:"application_agree_rate"`
	ReceivedOliveTotal        int                          `json:"received_olive_total"`
	ReceivedOliveReadCount    int                          `json:"received_olive_read_count"`
	ReceivedOliveHandledCount int                          `json:"received_olive_handled_count"`
	ReceivedOliveReadRate     float64                      `json:"received_olive_read_rate"`
	ReceivedOliveHandleRate   float64                      `json:"received_olive_handle_rate"`
	SourceStats               struct {
		FromList  int `json:"from_list"`
		FromShare int `json:"from_share"`
		Unknown   int `json:"unknown"`
	} `json:"source_stats"`
}

// GetTalentDashboard returns aggregated stats for the talent dashboard.
func (s *TalentProfileService) GetTalentDashboard(ctx context.Context, talentID, requesterUserID, days int) (*TalentDashboardResult, error) {
	isOwner, err := s.repo.TalentProfile.IsOwner(ctx, talentID, requesterUserID)
	if err != nil {
		log.Printf("[TalentProfileService.GetTalentDashboard] ownership check error: %v", err)
		return nil, ErrInternal("检查权限失败")
	}
	if !isOwner {
		return nil, ErrForbidden("仅名片主人可查看")
	}

	raw, err := s.repo.TalentViewLog.GetDashboardStats(ctx, talentID)
	if err != nil {
		log.Printf("[TalentProfileService.GetTalentDashboard] stats query error: %v", err)
		return nil, ErrInternal("获取看板数据失败")
	}

	result := &TalentDashboardResult{
		TotalViews:         raw.TotalViews,
		TodayViews:         raw.TodayViews,
		AvgDurationSeconds: raw.AvgDurationSeconds,
		HourlyViews:        raw.HourlyViews,
		VisitCount:         raw.TotalViews,
	}
	result.SourceStats.FromList = raw.FromList
	result.SourceStats.FromShare = raw.FromShare
	result.SourceStats.Unknown = raw.Unknown
	counts, err := s.repo.Interaction.CountsSince(ctx, repository.InteractionTalent, talentID, days)
	if err != nil {
		return nil, ErrInternal("get interaction dashboard failed")
	}
	result.LikeCount, result.FavoriteCount, result.ShareCount = counts.LikeCount, counts.FavoriteCount, counts.ShareCount
	unread, err := s.repo.Interaction.UnreadForTarget(ctx, repository.InteractionTalent, talentID, requesterUserID)
	if err != nil {
		return nil, ErrInternal("get interaction unread failed")
	}
	result.InteractionUnread = unread
	visitUnread, err := s.repo.TalentViewLog.CountUnreadVisits(ctx, talentID, requesterUserID)
	if err != nil {
		return nil, ErrInternal("get visit unread failed")
	}
	result.VisitUnreadCount = visitUnread
	result.InteractionUnread.VisitCount = visitUnread
	result.InteractionUnread.TotalCount += visitUnread

	applicationStats, err := s.repo.Application.GetUserDashboardStats(ctx, raw.OwnerUserID)
	if err != nil {
		return nil, ErrInternal("get application dashboard failed")
	}
	result.AppliedProjectTotal = applicationStats.Total
	result.ApplicationReadRate = dashboardRate(applicationStats.Read, applicationStats.Total)
	result.ApplicationAgreeRate = dashboardRate(applicationStats.Approved, applicationStats.Read)

	oliveStats, err := s.repo.OliveBranch.GetUserReceivedDashboardStats(ctx, raw.OwnerUserID)
	if err != nil {
		return nil, ErrInternal("get received olive dashboard failed")
	}
	result.ReceivedOliveTotal = oliveStats.Total
	result.ReceivedOliveReadCount = oliveStats.Read
	result.ReceivedOliveHandledCount = oliveStats.Handled
	result.ReceivedOliveReadRate = dashboardRate(oliveStats.Read, oliveStats.Total)
	result.ReceivedOliveHandleRate = dashboardRate(oliveStats.Handled, oliveStats.Total)
	return result, nil
}

// RecordTalentViewDuration inserts a dwell-time entry for a talent profile.
func (s *TalentProfileService) RecordTalentViewDuration(ctx context.Context, talentID, viewerUserID, durationMs int) error {
	if durationMs <= 0 || durationMs > 3_600_000 {
		return nil
	}
	var uidPtr *int
	if viewerUserID > 0 {
		uid := viewerUserID
		uidPtr = &uid
	}
	if err := s.repo.TalentViewLog.InsertDurationLog(ctx, talentID, uidPtr, durationMs); err != nil {
		log.Printf("[TalentProfileService.RecordTalentViewDuration] error: %v", err)
		return ErrInternal("上报停留时长失败")
	}
	return nil
}

// TalentViewersResult is the payload for GET /talent-profiles/{id}/viewers.
type TalentViewersResult struct {
	Total int
	List  []repository.TalentViewer
}

// GetTalentViewers returns authenticated viewers of the talent profile in the last 24 h.
func (s *TalentProfileService) GetTalentViewers(ctx context.Context, talentID, requesterUserID, limit int) (*TalentViewersResult, error) {
	isOwner, err := s.repo.TalentProfile.IsOwner(ctx, talentID, requesterUserID)
	if err != nil {
		log.Printf("[TalentProfileService.GetTalentViewers] ownership check error: %v", err)
		return nil, ErrInternal("检查权限失败")
	}
	if !isOwner {
		return nil, ErrForbidden("仅名片主人可查看")
	}

	viewers, total, err := s.repo.TalentViewLog.GetViewers(ctx, talentID, limit)
	if err != nil {
		log.Printf("[TalentProfileService.GetTalentViewers] query error: %v", err)
		return nil, ErrInternal("获取访客记录失败")
	}

	return &TalentViewersResult{Total: total, List: viewers}, nil
}

// TopTalentViewersResult is the payload for GET /talent-profiles/{id}/top-viewers.
type TopTalentViewersResult struct {
	List []repository.TopTalentViewer `json:"list"`
}

// GetTopTalentViewers returns today's most frequent authenticated viewers.
func (s *TalentProfileService) GetTopTalentViewers(ctx context.Context, talentID, requesterUserID, limit int) (*TopTalentViewersResult, error) {
	isOwner, err := s.repo.TalentProfile.IsOwner(ctx, talentID, requesterUserID)
	if err != nil {
		log.Printf("[TalentProfileService.GetTopTalentViewers] ownership check error: %v", err)
		return nil, ErrInternal("检查权限失败")
	}
	if !isOwner {
		return nil, ErrForbidden("仅名片主人可查看")
	}

	viewers, err := s.repo.TalentViewLog.GetTopViewersToday(ctx, talentID, limit)
	if err != nil {
		log.Printf("[TalentProfileService.GetTopTalentViewers] query error: %v", err)
		return nil, ErrInternal("获取高频访客失败")
	}
	return &TopTalentViewersResult{List: viewers}, nil
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

	// 异步发送审核结果通知
	go func(asyncCtx context.Context, userID int) {
		user, err := s.repo.User.GetByID(asyncCtx, userID)
		if err != nil || user == nil {
			log.Printf("[TalentProfileService.ReviewTalentProfile] get user for notification error: %v", err)
			return
		}

		resultStr := "审核通过"
		remark := "名片已上架人才库，快去看看吧！"
		if status == models.TalentStatusPrivate {
			resultStr = "审核拒绝"
			remark = "请完善名片资料后重新提交。"
		}

		userName := "同学"
		if user.Nickname != nil && *user.Nickname != "" {
			userName = *user.Nickname
		}

		data := map[string]string{
			"user_name": userName,
			"result":    resultStr,
			"remark":    remark,
		}

		if err := s.message.SendSubscribeMsgByBizKey(asyncCtx, userID, models.MsgBizKeyAuditResultUser, data); err != nil {
			log.Printf("[TalentProfileService.ReviewTalentProfile] notification error: %v", err)
		}
	}(context.WithoutCancel(ctx), profile.UserID)

	return nil
}
