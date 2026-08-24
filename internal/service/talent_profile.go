package service

import (
	"context"
	"log"
	"strings"
	"time"

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

type talentViewRecorder interface {
	RecordView(ctx context.Context, log *models.TalentViewLog) error
}

// NewTalentProfileService creates a new TalentProfileService.
func NewTalentProfileService(repo *repository.Repository, contentAudit *ContentAuditService, message *MessageService) *TalentProfileService {
	return &TalentProfileService{repo: repo, contentAudit: contentAudit, message: message}
}

// resolveUpsertStatus determines the actual status to save based on the requested status and the current status.
//
// Rules:
//   - Brand-new profiles default to Reviewing (2) when no status is requested.
//     An explicit Private (0) request remains private, while Online (1) is
//     demoted to Reviewing (2) to prevent self-approval.
//   - If no status is requested, Online (1) moves to Reviewing (2) after an edit;
//     otherwise the existing status is preserved.
//   - An explicit Online (1) request is demoted to Reviewing (2) to prevent self-approval.
//   - Any other valid explicit status is used as-is for an existing profile.
func (s *TalentProfileService) resolveUpsertStatus(requestedStatus *api.TalentStatus, currentStatus *int) (*int, error) {
	if requestedStatus == nil {
		// Pure content edit — apply automatic status transition.
		if currentStatus != nil && *currentStatus == models.TalentStatusOnline {
			// 已上架 → 编辑后进入待审核，管理员重新审核后再上架
			resolved := models.TalentStatusReviewing
			return &resolved, nil
		}
		// 待审核或已下架 → 保持原状态不变；全新档案 → 默认审核中
		if currentStatus != nil {
			return currentStatus, nil
		}
		resolved := models.TalentStatusReviewing
		return &resolved, nil
	}

	statusInt := int(*requestedStatus)
	if err := IsValidStatus("talent_profile.status", statusInt); err != nil {
		return nil, err
	}
	if currentStatus == nil {
		if statusInt == models.TalentStatusOnline {
			resolved := models.TalentStatusReviewing
			return &resolved, nil
		}
		return &statusInt, nil
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
	if existing == nil {
		user, userErr := s.repo.User.GetByID(ctx, userID)
		if userErr != nil {
			log.Printf("[TalentProfileService.UpsertTalentProfile] repository error getting user: %v", userErr)
			return nil, ErrInternal("获取用户信息失败")
		}
		if user == nil {
			return nil, ErrNotFound("用户不存在")
		}
		if user.SchoolID == nil || *user.SchoolID <= 0 || user.MajorID == nil || *user.MajorID <= 0 {
			return nil, ErrBadRequest("请先填写学校和专业")
		}
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
	var removedWorkImageKeys []string
	if req.WorkImageKeys != nil {
		if s.repo.Media == nil {
			return nil, ErrInternal("talent media repository unavailable")
		}
		removedWorkImageKeys, err = s.repo.Media.ReplaceTalentWorkImages(ctx, userID, profile.ID, *req.WorkImageKeys)
		if err != nil {
			log.Printf("[TalentProfileService.UpsertTalentProfile] save work images error: %v", err)
			return nil, ErrBadRequest("作品图片无效")
		}
	}

	updated, err := s.repo.TalentProfile.GetByUserID(ctx, userID)
	if err != nil {
		log.Printf("[TalentProfileService.UpsertTalentProfile] repository error reloading: %v", err)
		return nil, ErrInternal("获取人才档案失败")
	}
	if updated == nil {
		return nil, ErrNotFound("人才档案不存在")
	}
	updated.RemovedWorkImageKeys = removedWorkImageKeys

	return updated, nil
}

// SetTalentProfilePrivate hides the current user's talent profile without deleting it.
// When expectedStatus is provided, the transition only succeeds if the status
// has not changed since the client loaded the profile.
func (s *TalentProfileService) SetTalentProfilePrivate(ctx context.Context, userID int, expectedStatus *int) error {
	profile, err := s.repo.TalentProfile.GetByUserID(ctx, userID)
	if err != nil {
		log.Printf("[TalentProfileService.SetTalentProfilePrivate] repository error getting profile: %v", err)
		return ErrInternal("获取人才档案失败")
	}
	if profile == nil {
		return ErrNotFound("人才档案不存在")
	}
	if profile.Status == nil {
		return ErrBadRequest("人才名片状态异常")
	}
	currentStatus := *profile.Status
	if expectedStatus != nil {
		if *expectedStatus != models.TalentStatusOnline && *expectedStatus != models.TalentStatusReviewing {
			return ErrBadRequest("无效的人才名片预期状态")
		}
		if currentStatus != *expectedStatus {
			return ErrBadRequest("人才名片状态已变化，请刷新后重试")
		}
	}
	if currentStatus == models.TalentStatusPrivate {
		return nil
	}

	updated, err := s.repo.TalentProfile.UpdateStatusIfCurrent(
		ctx,
		profile.ID,
		currentStatus,
		models.TalentStatusPrivate,
		nil,
	)
	if err != nil {
		log.Printf("[TalentProfileService.SetTalentProfilePrivate] repository error updating status: %v", err)
		return ErrInternal("下架人才档案失败")
	}
	if !updated {
		return ErrBadRequest("人才名片状态已变化，请刷新后重试")
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

// GetTalentProfileWithView returns a talent profile and records a real view.
func (s *TalentProfileService) GetTalentProfileWithView(ctx context.Context, id, viewerUserID, source int) (*models.TalentProfile, error) {
	profile, err := s.GetTalentProfile(ctx, id)
	if err != nil {
		return nil, err
	}

	var uidPtr *int
	if viewerUserID > 0 {
		uid := viewerUserID
		uidPtr = &uid
	}
	entry := &models.TalentViewLog{TalentID: id, UserID: uidPtr, Source: source}
	var recordErr error
	if recorder, ok := s.repo.TalentViewLog.(talentViewRecorder); ok {
		recordErr = recorder.RecordView(ctx, entry)
	} else {
		recordErr = s.repo.TalentViewLog.InsertViewLog(ctx, entry)
		if recordErr == nil {
			recordErr = s.repo.TalentProfile.IncrementViewCount(ctx, id)
		}
	}
	if recordErr != nil {
		log.Printf("[TalentProfileService.GetTalentProfileWithView] record view error (non-fatal): %v", recordErr)
		return profile, nil
	}
	if viewerUserID <= 0 || viewerUserID == profile.UserID || s.message == nil {
		return profile, nil
	}

	go func(parentCtx context.Context) {
		asyncCtx, cancel := context.WithTimeout(context.WithoutCancel(parentCtx), 10*time.Second)
		defer cancel()
		progress, err := s.repo.TalentViewLog.NotifyProgress(asyncCtx, id, viewerUserID, profile.UserID)
		if err != nil {
			log.Printf("[TalentProfileService.GetTalentProfileWithView] get visit notify progress error (non-fatal): %v", err)
			return
		}
		if !shouldSendGroupedInteractionNotification(progress) {
			return
		}
		viewer, err := s.repo.User.GetByID(asyncCtx, viewerUserID)
		if err != nil {
			log.Printf("[TalentProfileService.GetTalentProfileWithView] get viewer error (non-fatal): %v", err)
		}
		notification, ok := buildTalentVisitNotification(viewerUserID, profile, notificationUserName(viewer), progress.DistinctUserCount)
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
func (s *TalentProfileService) GetTalentViewers(ctx context.Context, talentID, requesterUserID, page, limit int) (*TalentViewersResult, error) {
	page, limit = normalizeViewerPage(page, limit)
	isOwner, err := s.repo.TalentProfile.IsOwner(ctx, talentID, requesterUserID)
	if err != nil {
		log.Printf("[TalentProfileService.GetTalentViewers] ownership check error: %v", err)
		return nil, ErrInternal("检查权限失败")
	}
	if !isOwner {
		return nil, ErrForbidden("仅名片主人可查看")
	}

	viewers, total, err := s.repo.TalentViewLog.GetViewers(ctx, talentID, page, limit)
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
func (s *TalentProfileService) TakedownTalentProfile(ctx context.Context, id int, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ErrBadRequest("下架原因不能为空")
	}

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

	if err := s.repo.TalentProfile.UpdateStatus(ctx, id, models.TalentStatusPrivate, &reason); err != nil {
		log.Printf("[TalentProfileService.TakedownTalentProfile] repository error updating status: %v", err)
		return ErrInternal("下架失败")
	}

	s.notifyTalentReviewResult(ctx, profile.UserID, "名片下架", truncate20WithEllipsis(reason))

	return nil
}

// ReviewTalentProfile reviews a talent profile from reviewing to approved or private.
func (s *TalentProfileService) ReviewTalentProfile(ctx context.Context, id, status int, rejectReason *string) error {
	var cleanedReason *string
	if rejectReason != nil {
		reason := strings.TrimSpace(*rejectReason)
		if reason != "" {
			cleanedReason = &reason
		}
	}
	if status == models.TalentStatusPrivate && cleanedReason == nil {
		return ErrBadRequest("驳回原因不能为空")
	}

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

	updated, err := s.repo.TalentProfile.UpdateStatusIfCurrent(
		ctx,
		id,
		models.TalentStatusReviewing,
		status,
		cleanedReason,
	)
	if err != nil {
		log.Printf("[TalentProfileService.ReviewTalentProfile] repository error updating status: %v", err)
		return ErrInternal("审核失败")
	}
	if !updated {
		return ErrBadRequest("当前人才档案状态不允许审核")
	}

	userID := profile.UserID
	user, err := s.repo.User.GetByID(context.WithoutCancel(ctx), userID)
	if err != nil || user == nil {
		log.Printf("[TalentProfileService.ReviewTalentProfile] get user for notification error: %v", err)
	} else {
		resultStr := "审核通过"
		remark := "名片已上架人才库，快去看看吧！"
		if status == models.TalentStatusPrivate {
			resultStr = "审核拒绝"
			remark = truncate20WithEllipsis(*cleanedReason)
		}

		userName := "同学"
		if user.Nickname != nil && *user.Nickname != "" {
			userName = truncate20(*user.Nickname)
		}

		data := map[string]string{
			"user_name": userName,
			"result":    resultStr,
			"remark":    remark,
		}

		if queueErr := s.message.SendSubscribeMsgByBizKey(context.WithoutCancel(ctx), userID, models.MsgBizKeyAuditResultUser, data); queueErr != nil {
			log.Printf("[TalentProfileService.ReviewTalentProfile] queue notification error: %v", queueErr)
		}
	}

	return nil
}

func (s *TalentProfileService) notifyTalentReviewResult(ctx context.Context, userID int, resultStr string, remark string) {
	user, err := s.repo.User.GetByID(context.WithoutCancel(ctx), userID)
	if err != nil || user == nil {
		log.Printf("[TalentProfileService.notifyTalentReviewResult] get user for notification error: %v", err)
		return
	}

	userName := "同学"
	if user.Nickname != nil && *user.Nickname != "" {
		userName = truncate20(*user.Nickname)
	}

	data := map[string]string{
		"user_name": userName,
		"result":    resultStr,
		"remark":    remark,
	}

	if queueErr := s.message.SendSubscribeMsgByBizKey(context.WithoutCancel(ctx), userID, models.MsgBizKeyAuditResultUser, data); queueErr != nil {
		log.Printf("[TalentProfileService.notifyTalentReviewResult] queue notification error: %v", queueErr)
	}
}
