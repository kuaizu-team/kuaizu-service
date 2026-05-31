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
	ProjectID           *int
}

func (s *SmsNoticeService) Send(ctx context.Context, userID int, input SendSmsNoticeInput) (*models.SmsNotice, error) {
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
	if existing != nil {
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

	notice := s.prepareNotice(&models.SmsNotice{}, branch, order, project, receiver)
	if err := s.repo.SmsNotice.Create(ctx, notice); err != nil {
		log.Printf("[SmsNoticeService] create sms notice: %v", err)
		return nil, ErrInternal("create sms notice failed")
	}
	if !notice.CreatedAt.IsZero() {
		return notice, nil
	}
	s.startAsyncSubmission(notice)
	fresh, err := s.repo.SmsNotice.GetByID(ctx, notice.ID)
	if err == nil && fresh != nil {
		return fresh, nil
	}
	return notice, nil
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
