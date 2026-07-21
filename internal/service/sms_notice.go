package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/messagecenter"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

const smsNoticeScene = "olive_branch_sms_notice"

type smsNoticeSubmitter interface {
	SubmitSmsNotice(ctx context.Context, req messagecenter.SmsNoticeRequest) (*messagecenter.SmsNoticeResponse, error)
}

type SmsNoticeService struct {
	repo                 *repository.Repository
	mu                   sync.RWMutex
	messageCenter        smsNoticeSubmitter
	messageCenterInitErr error
	messageCenterFactory func() (*messagecenter.Client, error)
}

func NewSmsNoticeService(repo *repository.Repository, messageCenter *messagecenter.Client, messageCenterInitErr error) *SmsNoticeService {
	svc := &SmsNoticeService{
		repo:                 repo,
		messageCenterInitErr: messageCenterInitErr,
		messageCenterFactory: messagecenter.NewClientFromEnv,
	}
	if messageCenter != nil {
		svc.messageCenter = messageCenter
	}
	return svc
}

type SendSmsNoticeInput struct {
	OrderID             int
	ReceiverUserID      int
	OliveBranchRecordID int
	ApplicationID       *int
	MemberRemovalID     *int64
	NoticeType          *string
	ProjectID           *int
}

type applicationSmsSubmitter interface {
	SubmitApplicationSms(ctx context.Context, req messagecenter.ApplicationSmsRequest) error
}

func (s *SmsNoticeService) Send(ctx context.Context, userID int, input SendSmsNoticeInput) (*models.SmsNotice, error) {
	if input.ApplicationID != nil {
		return s.sendApplicationSms(ctx, userID, input)
	}
	if input.MemberRemovalID != nil {
		return s.sendMemberRemovalSms(ctx, userID, input)
	}
	if input.NoticeType != nil {
		return s.sendOliveOutcomeSms(ctx, userID, input)
	}
	if input.OrderID <= 0 || input.ReceiverUserID <= 0 || input.OliveBranchRecordID <= 0 {
		return nil, ErrBadRequest("invalid sms notice parameters")
	}

	branch, err := s.repo.OliveBranch.GetByID(ctx, input.OliveBranchRecordID)
	if err != nil {
		return nil, ErrInternal("get olive branch failed")
	}
	if branch == nil {
		return nil, ErrNotFound("olive branch record not found")
	}
	if branch.SenderID != userID {
		return nil, ErrForbidden("no permission to send sms notice for this olive branch")
	}
	if branch.ReceiverID != input.ReceiverUserID {
		return nil, ErrBadRequest("receiverUserId does not match olive branch receiver")
	}
	if branch.Status != models.OliveBranchStatusPending {
		return nil, ErrBadRequest("sms notice is only available for a pending olive branch")
	}

	order, err := s.repo.Order.GetByID(ctx, input.OrderID)
	if err != nil {
		return nil, ErrInternal("get order failed")
	}
	if order == nil {
		return nil, ErrNotFound("order not found")
	}
	if order.UserID != userID {
		return nil, ErrForbidden("no permission to use this order")
	}
	if order.Status != models.OrderStatusPaid {
		return nil, ErrBadRequest("order is not paid")
	}

	product, err := s.repo.Product.GetByID(ctx, order.ProductID)
	if err != nil {
		return nil, ErrInternal("get product failed")
	}
	if product == nil || !isSmsNoticeProduct(product) {
		return nil, ErrBadRequest("order product is not sms notice")
	}

	projectID := branch.RelatedProjectID
	if input.ProjectID != nil {
		if *input.ProjectID != branch.RelatedProjectID {
			return nil, ErrBadRequest("projectId does not match olive branch project")
		}
		projectID = *input.ProjectID
	}

	project, err := s.repo.Project.GetByID(ctx, projectID)
	if err != nil {
		return nil, ErrInternal("get project failed")
	}
	if project == nil {
		return nil, ErrNotFound("project not found")
	}

	receiver, err := s.repo.User.GetByID(ctx, input.ReceiverUserID)
	if err != nil {
		return nil, ErrInternal("get receiver failed")
	}
	if receiver == nil {
		return nil, ErrNotFound("receiver not found")
	}

	existing, err := s.repo.SmsNotice.GetByOliveBranchRecordID(ctx, input.OliveBranchRecordID)
	if err != nil {
		return nil, ErrInternal("check sms notice failed")
	}
	// Resending advances the olive branch timestamp and starts a new interaction cycle.
	if existing != nil && !existing.CreatedAt.Before(branch.UpdatedAt) {
		return s.handleExistingNotice(ctx, existing, input, branch, order, project, receiver)
	}

	existingByOrder, err := s.repo.SmsNotice.GetByOrderID(ctx, input.OrderID)
	if err != nil {
		return nil, ErrInternal("check sms notice order failed")
	}
	if existingByOrder != nil {
		if existingByOrder.OliveBranchRecordID != input.OliveBranchRecordID {
			return nil, ErrBadRequest("order has already been used for another sms notice")
		}
		return s.handleExistingNotice(ctx, existingByOrder, input, branch, order, project, receiver)
	}

	notice := s.prepareNotice(&models.SmsNotice{}, branch, order, project, receiver)
	if err := s.repo.SmsNotice.Create(ctx, notice); err != nil {
		log.Printf("[SmsNoticeService] create sms notice: %v", err)
		return nil, ErrInternal("create sms notice failed")
	}
	if notice.OliveBranchRecordID != input.OliveBranchRecordID {
		return nil, ErrBadRequest("order has already been used for another sms notice")
	}
	if !notice.CreatedAt.IsZero() {
		return notice, nil
	}
	if err := s.recordOrderSmsTemplate(ctx, order.ID, "OLIVE_BRANCH_SMS_NOTICE"); err != nil {
		return nil, err
	}
	s.startAsyncSubmission(notice)
	fresh, err := s.repo.SmsNotice.GetByID(ctx, notice.ID)
	if err == nil && fresh != nil {
		return fresh, nil
	}
	return notice, nil
}

func (s *SmsNoticeService) sendOliveOutcomeSms(ctx context.Context, userID int, input SendSmsNoticeInput) (*models.SmsNotice, error) {
	noticeType := strings.TrimSpace(valueOrEmpty(input.NoticeType))
	if input.OrderID <= 0 || input.ReceiverUserID <= 0 || input.OliveBranchRecordID <= 0 ||
		(noticeType != "accepted" && noticeType != "rejected" && noticeType != "talent_rejected") {
		return nil, ErrBadRequest("invalid olive branch result sms parameters")
	}
	branch, err := s.repo.OliveBranch.GetByID(ctx, input.OliveBranchRecordID)
	if err != nil || branch == nil {
		return nil, ErrNotFound("olive branch record not found")
	}
	smsReceiverID := branch.ReceiverID
	if noticeType == "talent_rejected" {
		smsReceiverID = branch.SenderID
		if branch.ReceiverID != userID || branch.SenderID != input.ReceiverUserID {
			return nil, ErrForbidden("no permission to send olive branch result sms")
		}
	} else if branch.SenderID != userID || branch.ReceiverID != input.ReceiverUserID {
		return nil, ErrForbidden("no permission to send olive branch result sms")
	}
	expectedStatus := models.OliveBranchStatusRejected
	if noticeType == "accepted" {
		expectedStatus = models.OliveBranchStatusAccepted
	}
	if branch.Status != expectedStatus {
		return nil, ErrBadRequest("olive branch status does not match noticeType")
	}
	order, err := s.repo.Order.GetByID(ctx, input.OrderID)
	if err != nil || order == nil || order.UserID != userID || order.Status != models.OrderStatusPaid {
		return nil, ErrBadRequest("paid sms order is unavailable")
	}
	product, err := s.repo.Product.GetByID(ctx, order.ProductID)
	if err != nil || product == nil || !isSmsNoticeProduct(product) {
		return nil, ErrBadRequest("order product is not sms notice")
	}
	project, err := s.repo.Project.GetByID(ctx, branch.RelatedProjectID)
	if err != nil || project == nil {
		return nil, ErrNotFound("project not found")
	}
	receiver, err := s.repo.User.GetByID(ctx, smsReceiverID)
	if err != nil || receiver == nil || receiver.Phone == nil || strings.TrimSpace(*receiver.Phone) == "" {
		return nil, ErrBadRequest("receiver phone is unavailable")
	}
	nickname := models.DefaultUserNickname
	nicknameUser := receiver
	if noticeType == "talent_rejected" {
		nicknameUser, err = s.repo.User.GetByID(ctx, userID)
		if err != nil || nicknameUser == nil {
			return nil, ErrInternal("get sender failed")
		}
	}
	if nicknameUser.Nickname != nil && strings.TrimSpace(*nicknameUser.Nickname) != "" {
		nickname = strings.TrimSpace(*nicknameUser.Nickname)
	}
	teamRole := applicationSmsRoleName(valueOrEmpty(branch.OperatorRole))
	submitter, initErr, _ := s.resolveMessageCenter()
	if initErr != nil {
		return nil, ErrInternal("message center unavailable")
	}
	applicationSubmitter, ok := submitter.(applicationSmsSubmitter)
	if !ok {
		return nil, ErrInternal("message center client does not support result sms")
	}
	templateCode := "OLIVE_BRANCH_REJECTED"
	if noticeType == "accepted" {
		templateCode = "OLIVE_BRANCH_ACCEPTED"
	} else if noticeType == "talent_rejected" {
		templateCode = "OLIVE_BRANCH_TALENT_REJECTED"
	}
	taskKey := fmt.Sprintf("OLIVE_BRANCH_RESULT_SMS:%d:%s", input.OrderID, noticeType)
	if used, err := s.repo.SmsNotice.GetByOrderID(ctx, input.OrderID); err != nil {
		return nil, ErrInternal("check sms notice order failed")
	} else if used != nil {
		return nil, ErrBadRequest("order has already been used for another sms notice")
	}
	now := time.Now()
	channel, tag, trace := "SMS", "olive_branch_result_sms_"+noticeType, taskKey
	notice := &models.SmsNotice{Channel: &channel, BusinessTag: &tag, TraceID: &trace, OrderID: input.OrderID,
		OliveBranchRecordID: branch.ID, ProjectID: &branch.RelatedProjectID, SenderID: userID, ReceiverID: smsReceiverID,
		Status: models.SmsNoticeStatusPending, StartedAt: &now}
	outcomeRepo, ok := s.repo.SmsNotice.(interface {
		CreateOutcome(context.Context, *models.SmsNotice) error
	})
	if !ok {
		return nil, ErrInternal("sms repository does not support result records")
	}
	if err := outcomeRepo.CreateOutcome(ctx, notice); err != nil {
		return nil, ErrInternal("create olive branch result sms record failed")
	}
	if err := s.recordOrderSmsTemplate(ctx, order.ID, templateCode); err != nil {
		return nil, err
	}
	if err := applicationSubmitter.SubmitApplicationSms(ctx, messagecenter.ApplicationSmsRequest{
		TaskKey: taskKey, TemplateCode: templateCode, Phone: strings.TrimSpace(*receiver.Phone), Nickname: nickname,
		ProjectTitle: project.Name, TeamRole: teamRole, BusinessTag: "olive_branch_result_sms_" + noticeType, TraceID: taskKey,
	}); err != nil {
		notice.Status = models.SmsNoticeStatusFailed
		message := err.Error()
		notice.ErrorMessage = &message
		_ = s.repo.SmsNotice.Update(ctx, notice)
		return nil, ErrInternal("submit olive branch result sms failed")
	}
	notice.Status = models.SmsNoticeStatusCompleted
	notice.CompletedAt = &now
	if err := s.repo.SmsNotice.Update(ctx, notice); err != nil {
		return nil, ErrInternal("complete olive branch result sms record failed")
	}
	return notice, nil
}

func (s *SmsNoticeService) sendMemberRemovalSms(ctx context.Context, userID int, input SendSmsNoticeInput) (*models.SmsNotice, error) {
	if input.MemberRemovalID == nil || *input.MemberRemovalID <= 0 || input.OrderID <= 0 || input.ReceiverUserID <= 0 {
		return nil, ErrBadRequest("invalid member removal sms parameters")
	}
	var removal struct {
		ID          int64  `db:"id"`
		UserID      int    `db:"user_id"`
		ProjectID   int    `db:"project_id"`
		OperatorID  int    `db:"operator_id"`
		Role        string `db:"role"`
		ProjectName string `db:"project_name"`
	}
	if err := s.repo.DB().GetContext(ctx, &removal, `SELECT pmr.id,pmr.user_id,pmr.project_id,pmr.operator_id,pmr.role,p.name AS project_name FROM project_member_removal pmr JOIN project p ON p.id=pmr.project_id WHERE pmr.id=?`, *input.MemberRemovalID); err != nil {
		return nil, ErrNotFound("member removal record not found")
	}
	if removal.OperatorID != userID || removal.UserID != input.ReceiverUserID {
		return nil, ErrForbidden("no permission to send member removal sms")
	}
	order, err := s.repo.Order.GetByID(ctx, input.OrderID)
	if err != nil || order == nil || order.UserID != userID || order.Status != models.OrderStatusPaid {
		return nil, ErrBadRequest("paid sms order is unavailable")
	}
	product, err := s.repo.Product.GetByID(ctx, order.ProductID)
	if err != nil || product == nil || !isSmsNoticeProduct(product) {
		return nil, ErrBadRequest("order product is not sms notice")
	}
	if used, err := s.repo.SmsNotice.GetByOrderID(ctx, input.OrderID); err != nil {
		return nil, ErrInternal("check sms notice order failed")
	} else if used != nil {
		return nil, ErrBadRequest("order has already been used for another sms notice")
	}
	receiver, err := s.repo.User.GetByID(ctx, removal.UserID)
	if err != nil || receiver == nil || receiver.Phone == nil || strings.TrimSpace(*receiver.Phone) == "" {
		return nil, ErrBadRequest("receiver phone is unavailable")
	}
	nickname := models.DefaultUserNickname
	if receiver.Nickname != nil && strings.TrimSpace(*receiver.Nickname) != "" {
		nickname = strings.TrimSpace(*receiver.Nickname)
	}
	submitter, initErr, _ := s.resolveMessageCenter()
	if initErr != nil {
		return nil, ErrInternal("message center unavailable")
	}
	applicationSubmitter, ok := submitter.(applicationSmsSubmitter)
	if !ok {
		return nil, ErrInternal("message center client does not support member removal sms")
	}
	taskKey := fmt.Sprintf("MEMBER_REMOVAL_SMS:%d", input.OrderID)
	now := time.Now()
	channel, tag, trace := "SMS", "member_removal_sms", taskKey
	notice := &models.SmsNotice{Channel: &channel, BusinessTag: &tag, TraceID: &trace, OrderID: input.OrderID,
		ProjectID: &removal.ProjectID, SenderID: userID, ReceiverID: removal.UserID,
		Status: models.SmsNoticeStatusPending, StartedAt: &now}
	memberRemovalRepo, ok := s.repo.SmsNotice.(interface {
		CreateMemberRemoval(context.Context, *models.SmsNotice, int64) error
		CompleteMemberRemoval(context.Context, *models.SmsNotice) error
	})
	if !ok || memberRemovalRepo.CreateMemberRemoval(ctx, notice, removal.ID) != nil {
		return nil, ErrInternal("create member removal sms record failed")
	}
	if err := s.recordOrderSmsTemplate(ctx, order.ID, "MEMBER_REMOVAL_THANKS"); err != nil {
		return nil, err
	}
	if err := applicationSubmitter.SubmitApplicationSms(ctx, messagecenter.ApplicationSmsRequest{
		TaskKey: taskKey, TemplateCode: "MEMBER_REMOVAL_THANKS", Phone: strings.TrimSpace(*receiver.Phone), Nickname: nickname,
		ProjectTitle: removal.ProjectName, TeamRole: applicationSmsRoleName(removal.Role), BusinessTag: tag, TraceID: trace,
	}); err != nil {
		notice.Status = models.SmsNoticeStatusFailed
		message := err.Error()
		notice.ErrorMessage = &message
		_ = memberRemovalRepo.CompleteMemberRemoval(ctx, notice)
		return nil, ErrInternal("submit member removal sms failed")
	}
	notice.Status = models.SmsNoticeStatusCompleted
	notice.CompletedAt = &now
	if err := memberRemovalRepo.CompleteMemberRemoval(ctx, notice); err != nil {
		return nil, ErrInternal("complete member removal sms record failed")
	}
	return notice, nil
}

func (s *SmsNoticeService) recordOrderSmsTemplate(ctx context.Context, orderID int, code string) error {
	names := map[string]string{
		"OLIVE_BRANCH_SMS_NOTICE":                "橄榄枝短信通知",
		"OLIVE_BRANCH_REJECTED":                  "橄榄枝婉拒通知",
		"OLIVE_BRANCH_ACCEPTED":                  "橄榄枝接受通知",
		"OLIVE_BRANCH_TALENT_REJECTED":           "人才婉拒项目橄榄枝",
		"PROJECT_APPLICATION_REJECTED":           "项目申请不合适通知",
		"PROJECT_APPLICATION_ACCEPTED":           "项目申请通过通知",
		"PROJECT_APPLICATION_APPLICANT_REJECTED": "投递名片主动不合适",
		"MEMBER_REMOVAL_THANKS":                  "管理团队移除感谢",
	}
	name := names[code]
	if name == "" {
		name = code
	}
	if s.repo.DB() == nil {
		return nil
	}
	if _, err := s.repo.DB().ExecContext(ctx, "UPDATE `order` SET template_code=?, template_name=?, updated_at=NOW() WHERE id=?", code, name, orderID); err != nil {
		log.Printf("[SmsNoticeService] update order sms template, order_id=%d template_code=%s: %v", orderID, code, err)
		return ErrInternal("update order sms template failed")
	}
	return nil
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (s *SmsNoticeService) sendApplicationSms(ctx context.Context, userID int, input SendSmsNoticeInput) (*models.SmsNotice, error) {
	if input.ApplicationID == nil || input.NoticeType == nil || input.OrderID <= 0 || input.ReceiverUserID <= 0 {
		return nil, ErrBadRequest("invalid application sms parameters")
	}
	noticeType := strings.TrimSpace(*input.NoticeType)
	if noticeType != "rejected" && noticeType != "accepted" && noticeType != "applicant_rejected" {
		return nil, ErrBadRequest("invalid noticeType")
	}
	app, err := s.repo.Application.GetByID(ctx, *input.ApplicationID)
	if err != nil {
		return nil, ErrInternal("get application failed")
	}
	if app == nil {
		return nil, ErrNotFound("application not found")
	}
	smsReceiverID := app.UserID
	if noticeType == "applicant_rejected" {
		if app.UserID != userID {
			return nil, ErrForbidden("no permission to send application sms")
		}
		if app.ReviewerID == nil || *app.ReviewerID != input.ReceiverUserID {
			return nil, ErrBadRequest("receiverUserId does not match application reviewer")
		}
		smsReceiverID = *app.ReviewerID
	} else {
		if app.UserID != input.ReceiverUserID {
			return nil, ErrBadRequest("receiverUserId does not match application")
		}
		if app.ReviewerID == nil || *app.ReviewerID != userID {
			return nil, ErrForbidden("no permission to send application sms")
		}
	}
	if input.ProjectID != nil && *input.ProjectID != app.ProjectID {
		return nil, ErrBadRequest("projectId does not match application")
	}
	if noticeType == "rejected" && app.Status != models.ApplicationStatusRejected {
		return nil, ErrBadRequest("application is not rejected")
	}
	if noticeType == "accepted" && app.Status != models.ApplicationStatusJoined {
		return nil, ErrBadRequest("application is not accepted")
	}
	if noticeType == "applicant_rejected" && app.Status != models.ApplicationStatusRejected {
		return nil, ErrBadRequest("application is not rejected")
	}
	if noticeType == "applicant_rejected" {
		isApplicantRejected, err := s.isApplicantInitiatedRejection(ctx, app)
		if err != nil {
			return nil, ErrInternal("check application rejection source failed")
		}
		if !isApplicantRejected {
			return nil, ErrBadRequest("application was not rejected by applicant")
		}
	}
	order, err := s.repo.Order.GetByID(ctx, input.OrderID)
	if err != nil {
		return nil, ErrInternal("get order failed")
	}
	if order == nil {
		return nil, ErrNotFound("order not found")
	}
	if order.UserID != userID {
		return nil, ErrForbidden("no permission to use this order")
	}
	if order.Status != models.OrderStatusPaid {
		return nil, ErrBadRequest("order is not paid")
	}
	if used, err := s.repo.SmsNotice.GetByOrderID(ctx, input.OrderID); err != nil {
		return nil, ErrInternal("check sms notice order failed")
	} else if used != nil {
		return nil, ErrBadRequest("order has already been used for another sms notice")
	}
	if order.TemplateCode != nil && strings.TrimSpace(*order.TemplateCode) != "" {
		return nil, ErrBadRequest("order has already been used for another sms notice")
	}
	product, err := s.repo.Product.GetByID(ctx, order.ProductID)
	if err != nil {
		return nil, ErrInternal("get product failed")
	}
	if product == nil || !isSmsNoticeProduct(product) {
		return nil, ErrBadRequest("order product is not sms notice")
	}
	project, err := s.repo.Project.GetByID(ctx, app.ProjectID)
	if err != nil {
		return nil, ErrInternal("get project failed")
	}
	if project == nil {
		return nil, ErrNotFound("project not found")
	}
	receiver, err := s.repo.User.GetByID(ctx, smsReceiverID)
	if err != nil {
		return nil, ErrInternal("get receiver failed")
	}
	if receiver == nil || receiver.Phone == nil || strings.TrimSpace(*receiver.Phone) == "" {
		return nil, ErrBadRequest("receiver phone is unavailable")
	}
	nickname := models.DefaultUserNickname
	nicknameUser := receiver
	if noticeType == "applicant_rejected" {
		nicknameUser, err = s.repo.User.GetByID(ctx, userID)
		if err != nil || nicknameUser == nil {
			return nil, ErrInternal("get sender failed")
		}
	}
	if nicknameUser.Nickname != nil && strings.TrimSpace(*nicknameUser.Nickname) != "" {
		nickname = strings.TrimSpace(*nicknameUser.Nickname)
	}
	teamRole := "团队成员"
	if noticeType == "accepted" && app.AssignedRole != nil && strings.TrimSpace(*app.AssignedRole) != "" {
		teamRole = applicationSmsRoleName(strings.TrimSpace(*app.AssignedRole))
	}
	if noticeType == "rejected" && app.ReviewerRole != nil && strings.TrimSpace(*app.ReviewerRole) != "" {
		teamRole = applicationSmsRoleName(strings.TrimSpace(*app.ReviewerRole))
	}
	if noticeType == "applicant_rejected" && app.ReviewerRole != nil && strings.TrimSpace(*app.ReviewerRole) != "" {
		teamRole = applicationSmsRoleName(strings.TrimSpace(*app.ReviewerRole))
	}
	submitter, initErr, _ := s.resolveMessageCenter()
	if initErr != nil {
		return nil, ErrInternal("message center unavailable")
	}
	applicationSubmitter, ok := submitter.(applicationSmsSubmitter)
	if !ok {
		return nil, ErrInternal("message center client does not support application sms")
	}
	templateCode := "PROJECT_APPLICATION_REJECTED"
	if noticeType == "accepted" {
		templateCode = "PROJECT_APPLICATION_ACCEPTED"
	} else if noticeType == "applicant_rejected" {
		templateCode = "PROJECT_APPLICATION_APPLICANT_REJECTED"
	}
	taskKey := fmt.Sprintf("PROJECT_APPLICATION_SMS:%d:%s", input.OrderID, noticeType)
	now := time.Now()
	channel, tag, trace := "SMS", "project_application_sms_"+noticeType, fmt.Sprintf("PROJECT_APPLICATION_SMS:%d", input.OrderID)
	content := fmt.Sprintf("PROJECT_APPLICATION_SMS:%d:%s", *input.ApplicationID, noticeType)
	notice := &models.SmsNotice{Channel: &channel, BusinessTag: &tag, TraceID: &trace, OrderID: input.OrderID,
		ProjectID: &app.ProjectID, SenderID: userID, ReceiverID: smsReceiverID, SmsContent: content,
		Status: models.SmsNoticeStatusPending, StartedAt: &now}
	applicationRepo, ok := s.repo.SmsNotice.(interface {
		CreateApplication(context.Context, *models.SmsNotice) error
	})
	if !ok {
		return nil, ErrInternal("sms repository does not support application records")
	}
	if err := applicationRepo.CreateApplication(ctx, notice); err != nil {
		return nil, ErrInternal("create application sms record failed")
	}
	if !notice.CreatedAt.IsZero() {
		return nil, ErrBadRequest("order has already been used for another sms notice")
	}
	if err := s.recordOrderSmsTemplate(ctx, order.ID, templateCode); err != nil {
		return nil, err
	}
	err = applicationSubmitter.SubmitApplicationSms(ctx, messagecenter.ApplicationSmsRequest{
		TaskKey: taskKey, TemplateCode: templateCode,
		Phone: strings.TrimSpace(*receiver.Phone), Nickname: nickname, ProjectTitle: project.Name, TeamRole: teamRole,
		BusinessTag: tag, TraceID: trace,
	})
	if err != nil {
		notice.Status = models.SmsNoticeStatusFailed
		message := err.Error()
		notice.ErrorMessage = &message
		notice.CompletedAt = &now
		_ = s.repo.SmsNotice.Update(ctx, notice)
		return nil, ErrInternal("submit application sms failed")
	}
	notice.Status = models.SmsNoticeStatusCompleted
	notice.CompletedAt = &now
	if err := s.repo.SmsNotice.Update(ctx, notice); err != nil {
		return nil, ErrInternal("complete application sms record failed")
	}
	return notice, nil
}

func (s *SmsNoticeService) isApplicantInitiatedRejection(ctx context.Context, app *models.ProjectApplication) (bool, error) {
	if app == nil || app.Status != models.ApplicationStatusRejected {
		return false, nil
	}
	if app.ApplicantRejected != nil {
		return *app.ApplicantRejected, nil
	}
	if s.repo == nil || s.repo.DB() == nil {
		return false, nil
	}
	var reviewerRejectedCount int
	if err := s.repo.DB().GetContext(ctx, &reviewerRejectedCount,
		`SELECT COUNT(*) FROM status_notification WHERE application_id = ? AND type = ?`,
		app.ID, models.StatusNotificationApplicationRejected,
	); err != nil {
		return false, err
	}
	return reviewerRejectedCount == 0, nil
}

func applicationSmsRoleName(code string) string {
	names := map[string]string{
		"TEAM_LEADER": "团队负责人", "RECRUITMENT_LEADER": "招募负责人",
		"TECH_LEADER": "技术负责人", "OPERATIONS_LEADER": "运营负责人",
		"PUBLICITY_LEADER": "宣传负责人", "DESIGN_LEADER": "美化负责人",
		"LEGAL_LEADER": "法务负责人", "TEAM_MEMBER": "团队成员",
		"LEARNING_MEMBER": "学习成员",
	}
	if name := names[code]; name != "" {
		return name
	}
	return "团队成员"
}

// RetryByOrder resends a failed SMS using the original paid order and business record.
func (s *SmsNoticeService) RetryByOrder(ctx context.Context, userID, orderID int) (*models.SmsNotice, error) {
	order, err := s.repo.Order.GetByID(ctx, orderID)
	if err != nil || order == nil {
		return nil, ErrNotFound("order not found")
	}
	if order.UserID != userID || order.Status != models.OrderStatusPaid {
		return nil, ErrForbidden("no permission to retry this order")
	}
	notice, err := s.repo.SmsNotice.GetByOrderID(ctx, orderID)
	if err != nil || notice == nil {
		return nil, ErrNotFound("sms notice not found")
	}
	if notice.SenderID != userID || notice.Status != models.SmsNoticeStatusFailed {
		return nil, ErrBadRequest("only a failed sms notice can be retried")
	}
	tag := valueOrEmpty(notice.BusinessTag)
	if tag == smsNoticeScene {
		return s.Send(ctx, userID, SendSmsNoticeInput{
			OrderID: orderID, ReceiverUserID: notice.ReceiverID,
			OliveBranchRecordID: notice.OliveBranchRecordID, ProjectID: notice.ProjectID,
		})
	}
	if order.TemplateCode == nil || strings.TrimSpace(*order.TemplateCode) == "" {
		return nil, ErrBadRequest("sms template is unavailable for retry")
	}
	receiver, err := s.repo.User.GetByID(ctx, notice.ReceiverID)
	if err != nil || receiver == nil || receiver.Phone == nil || strings.TrimSpace(*receiver.Phone) == "" {
		return nil, ErrBadRequest("receiver phone is unavailable")
	}
	projectTitle := "项目邀约"
	if notice.ProjectID != nil {
		project, projectErr := s.repo.Project.GetByID(ctx, *notice.ProjectID)
		if projectErr == nil && project != nil {
			projectTitle = project.Name
		}
	}
	nickname := displayName(receiver)
	teamRole := "团队成员"

	switch {
	case strings.HasPrefix(tag, "olive_branch_result_sms_"):
		branch, branchErr := s.repo.OliveBranch.GetByID(ctx, notice.OliveBranchRecordID)
		if branchErr != nil || branch == nil {
			return nil, ErrNotFound("olive branch record not found")
		}
		teamRole = applicationSmsRoleName(valueOrEmpty(branch.OperatorRole))
		if strings.HasSuffix(tag, "talent_rejected") {
			sender, senderErr := s.repo.User.GetByID(ctx, userID)
			if senderErr != nil || sender == nil {
				return nil, ErrInternal("get sender failed")
			}
			nickname = displayName(sender)
		}
	case strings.HasPrefix(tag, "project_application_sms_"):
		var applicationID int
		var noticeType string
		if _, scanErr := fmt.Sscanf(notice.SmsContent, "PROJECT_APPLICATION_SMS:%d:%s", &applicationID, &noticeType); scanErr != nil {
			return nil, ErrBadRequest("application sms metadata is unavailable")
		}
		application, applicationErr := s.repo.Application.GetByID(ctx, applicationID)
		if applicationErr != nil || application == nil {
			return nil, ErrNotFound("application not found")
		}
		if noticeType == "accepted" {
			teamRole = applicationSmsRoleName(valueOrEmpty(application.AssignedRole))
		} else {
			teamRole = applicationSmsRoleName(valueOrEmpty(application.ReviewerRole))
		}
		if noticeType == "applicant_rejected" {
			sender, senderErr := s.repo.User.GetByID(ctx, userID)
			if senderErr != nil || sender == nil {
				return nil, ErrInternal("get sender failed")
			}
			nickname = displayName(sender)
		}
	case tag == "member_removal_sms":
		if notice.MemberRemovalID == nil {
			return nil, ErrBadRequest("member removal metadata is unavailable")
		}
		var role string
		if dbErr := s.repo.DB().GetContext(ctx, &role, `SELECT role FROM project_member_removal WHERE id=?`, *notice.MemberRemovalID); dbErr != nil {
			return nil, ErrNotFound("member removal record not found")
		}
		teamRole = applicationSmsRoleName(role)
	default:
		return nil, ErrBadRequest("sms notice type does not support retry")
	}

	submitter, initErr, _ := s.resolveMessageCenter()
	if initErr != nil {
		return nil, ErrInternal("message center unavailable")
	}
	applicationSubmitter, ok := submitter.(applicationSmsSubmitter)
	if !ok {
		return nil, ErrInternal("message center client does not support sms retry")
	}
	now := time.Now()
	notice.Status = models.SmsNoticeStatusSending
	notice.ErrorMessage = nil
	notice.StartedAt = &now
	notice.CompletedAt = nil
	if err := s.repo.SmsNotice.Update(ctx, notice); err != nil {
		return nil, ErrInternal("update sms notice for retry failed")
	}
	taskKey := valueOrEmpty(notice.TraceID)
	if taskKey == "" {
		return nil, ErrBadRequest("sms task key is unavailable")
	}
	err = applicationSubmitter.SubmitApplicationSms(ctx, messagecenter.ApplicationSmsRequest{
		TaskKey: taskKey, TemplateCode: *order.TemplateCode, Phone: strings.TrimSpace(*receiver.Phone),
		Nickname: nickname, ProjectTitle: projectTitle, TeamRole: teamRole,
		BusinessTag: tag, TraceID: taskKey, Retry: true,
	})
	if err != nil {
		message := err.Error()
		notice.Status = models.SmsNoticeStatusFailed
		notice.ErrorMessage = &message
		notice.CompletedAt = &now
		_ = s.repo.SmsNotice.Update(ctx, notice)
		return nil, ErrInternal("retry sms notice failed")
	}
	notice.Status = models.SmsNoticeStatusCompleted
	notice.CompletedAt = &now
	if err := s.repo.SmsNotice.Update(ctx, notice); err != nil {
		return nil, ErrInternal("complete sms retry failed")
	}
	return notice, nil
}
func (s *SmsNoticeService) handleExistingNotice(ctx context.Context, existing *models.SmsNotice, input SendSmsNoticeInput, branch *models.OliveBranch, order *models.Order, project *models.Project, receiver *models.User) (*models.SmsNotice, error) {
	switch existing.Status {
	case models.SmsNoticeStatusCompleted, models.SmsNoticeStatusPending, models.SmsNoticeStatusSending:
		return existing, nil
	case models.SmsNoticeStatusFailed:
		if existing.OrderID != input.OrderID {
			return nil, ErrBadRequest("failed sms notice can only retry with the original paid order")
		}
		notice := s.prepareNotice(existing, branch, order, project, receiver)
		if err := s.repo.SmsNotice.Update(ctx, notice); err != nil {
			log.Printf("[SmsNoticeService] update failed notice for retry: %v", err)
			return nil, ErrInternal("update sms notice failed")
		}
		s.startAsyncSubmission(notice)
		return notice, nil
	default:
		return existing, nil
	}
}

func (s *SmsNoticeService) GetByID(ctx context.Context, userID, id int) (*models.SmsNotice, error) {
	notice, err := s.repo.SmsNotice.GetByID(ctx, id)
	if err != nil {
		return nil, ErrInternal("get sms notice failed")
	}
	return s.checkNoticeVisible(notice, userID)
}

func (s *SmsNoticeService) GetByOliveBranchRecordID(ctx context.Context, userID, oliveBranchRecordID int) (*models.SmsNotice, error) {
	notice, err := s.repo.SmsNotice.GetByOliveBranchRecordID(ctx, oliveBranchRecordID)
	if err != nil {
		return nil, ErrInternal("get sms notice failed")
	}
	if notice == nil {
		return nil, ErrNotFound("sms notice not found")
	}
	branch, err := s.repo.OliveBranch.GetByID(ctx, oliveBranchRecordID)
	if err != nil {
		return nil, ErrInternal("get olive branch failed")
	}
	if branch == nil {
		return nil, ErrNotFound("olive branch record not found")
	}
	if notice.CreatedAt.Before(branch.UpdatedAt) {
		return nil, ErrNotFound("sms notice not found")
	}
	return s.checkNoticeVisible(notice, userID)
}

func (s *SmsNoticeService) checkNoticeVisible(notice *models.SmsNotice, userID int) (*models.SmsNotice, error) {
	if notice == nil {
		return nil, ErrNotFound("sms notice not found")
	}
	if notice.SenderID != userID && notice.ReceiverID != userID {
		return nil, ErrForbidden("no permission to view this sms notice")
	}
	return notice, nil
}

func (s *SmsNoticeService) prepareNotice(notice *models.SmsNotice, branch *models.OliveBranch, order *models.Order, project *models.Project, receiver *models.User) *models.SmsNotice {
	channel := "SMS"
	businessTag := smsNoticeScene
	traceID := fmt.Sprintf("OLIVE_BRANCH_SMS:%d", order.ID)
	now := time.Now()
	projectID := project.ID
	content := fmt.Sprintf("【项目邀约】%s同学您好，您收到来自%s的橄榄枝邀请，请到快组校园微信小程序查看详情", displayName(receiver), project.Name)

	notice.Channel = &channel
	notice.BusinessTag = &businessTag
	notice.TraceID = &traceID
	notice.OrderID = order.ID
	notice.OliveBranchRecordID = branch.ID
	notice.ProjectID = &projectID
	notice.SenderID = branch.SenderID
	notice.ReceiverID = branch.ReceiverID
	notice.SmsContent = content
	notice.Status = models.SmsNoticeStatusSending
	notice.ErrorMessage = nil
	notice.Provider = nil
	notice.ProviderBizID = nil
	notice.StartedAt = &now
	notice.CompletedAt = nil
	return notice
}

func (s *SmsNoticeService) startAsyncSubmission(notice *models.SmsNotice) {
	req := messagecenter.SmsNoticeRequest{
		TraceID:             stringValue(notice.TraceID),
		NoticeID:            notice.ID,
		OrderID:             notice.OrderID,
		SenderUserID:        notice.SenderID,
		ReceiverUserID:      notice.ReceiverID,
		OliveBranchRecordID: notice.OliveBranchRecordID,
		ProjectID:           notice.ProjectID,
		Content:             notice.SmsContent,
	}
	go func() {
		submitter, initErr, baseURL := s.resolveMessageCenter()
		if initErr != nil {
			log.Printf("[SmsNoticeService] message center unavailable, notice_id=%d base_url_empty=%t: %v", notice.ID, baseURL == "", initErr)
			s.markFailed(notice, "message center is not configured: "+initErr.Error())
			return
		}
		if submitter == nil {
			s.markFailed(notice, "message center client is nil")
			return
		}

		var (
			resp *messagecenter.SmsNoticeResponse
			err  error
		)
		for attempt := 1; attempt <= 3; attempt++ {
			callCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			resp, err = submitter.SubmitSmsNotice(callCtx, req)
			cancel()
			if err == nil {
				break
			}
			log.Printf("[SmsNoticeService] submit sms notice failed, notice_id=%d attempt=%d: %v", notice.ID, attempt, err)
			if attempt < 3 {
				time.Sleep(time.Duration(attempt) * 200 * time.Millisecond)
			}
		}
		if err != nil {
			s.markFailed(notice, "submit message center failed: "+err.Error())
			return
		}
		if resp != nil && resp.Accepted != nil && !*resp.Accepted {
			fresh, getErr := s.repo.SmsNotice.GetByID(context.Background(), notice.ID)
			if getErr != nil {
				log.Printf("[SmsNoticeService] reload rejected sms notice failed, notice_id=%d: %v", notice.ID, getErr)
				return
			}
			if fresh != nil {
				*notice = *fresh
			}
			return
		}

		notice.Status = models.SmsNoticeStatusSending
		notice.CompletedAt = nil
		notice.ErrorMessage = nil
		if resp != nil {
			if resp.Provider != "" {
				notice.Provider = &resp.Provider
			}
			providerBizID := resp.ProviderBizID
			if providerBizID == "" {
				providerBizID = resp.TaskID
			}
			if providerBizID != "" {
				notice.ProviderBizID = &providerBizID
			}
		}
		if err := s.repo.SmsNotice.Update(context.Background(), notice); err != nil {
			log.Printf("[SmsNoticeService] update completed sms notice failed, notice_id=%d: %v", notice.ID, err)
		}
	}()
}

func (s *SmsNoticeService) resolveMessageCenter() (smsNoticeSubmitter, error, string) {
	s.mu.RLock()
	submitter := s.messageCenter
	initErr := s.messageCenterInitErr
	baseURL := smsMessageCenterBaseURL(submitter)
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
		return s.messageCenter, s.messageCenterInitErr, smsMessageCenterBaseURL(s.messageCenter)
	}
	s.messageCenter = client
	s.messageCenterInitErr = nil
	return s.messageCenter, nil, client.BaseURL()
}

func smsMessageCenterBaseURL(submitter smsNoticeSubmitter) string {
	if client, ok := submitter.(*messagecenter.Client); ok {
		return client.BaseURL()
	}
	return ""
}

func (s *SmsNoticeService) markFailed(notice *models.SmsNotice, message string) {
	now := time.Now()
	notice.Status = models.SmsNoticeStatusFailed
	notice.ErrorMessage = &message
	notice.CompletedAt = &now
	if err := s.repo.SmsNotice.Update(context.Background(), notice); err != nil {
		log.Printf("[SmsNoticeService] update failed sms notice failed, notice_id=%d: %v", notice.ID, err)
	}
}

func isSmsNoticeProduct(product *models.Product) bool {
	if product == nil {
		return false
	}
	if strings.TrimSpace(product.Name) == "短信通知" {
		return true
	}
	if product.ConfigJSON == nil {
		return false
	}
	var cfg struct {
		Scene string `json:"scene"`
	}
	if err := json.Unmarshal([]byte(*product.ConfigJSON), &cfg); err != nil {
		return false
	}
	return cfg.Scene == smsNoticeScene
}

func displayName(user *models.User) string {
	if user != nil && user.Nickname != nil && strings.TrimSpace(*user.Nickname) != "" {
		return strings.TrimSpace(*user.Nickname)
	}
	return "这位"
}
