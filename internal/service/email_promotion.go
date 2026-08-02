package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/messagecenter"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

const promotionSubmissionWorkers = 8

// EmailPromotionService handles email promotion business logic.
type EmailPromotionService struct {
	repo                 *repository.Repository
	orderPushClaimer     orderPushClaimRepository
	mu                   sync.RWMutex
	messageCenter        projectPromotionSubmitter
	messageCenterInitErr error
	messageCenterFactory func() (*messagecenter.Client, error)
	submissionSlots      chan struct{}
	submissionWG         sync.WaitGroup
}

type projectPromotionSubmitter interface {
	SubmitProjectPromotion(ctx context.Context, req messagecenter.ProjectPromotionRequest) (*messagecenter.ProjectPromotionResponse, error)
}

// NewEmailPromotionService creates a new EmailPromotionService.
func NewEmailPromotionService(repo *repository.Repository) *EmailPromotionService {
	return &EmailPromotionService{
		repo:                 repo,
		orderPushClaimer:     repo,
		messageCenterFactory: messagecenter.NewClientFromEnv,
		submissionSlots:      make(chan struct{}, promotionSubmissionWorkers),
	}
}

// NewEmailPromotionServiceWithMessageCenter creates a service with a message-center client.
func NewEmailPromotionServiceWithMessageCenter(repo *repository.Repository, messageCenter *messagecenter.Client, messageCenterInitErr error) *EmailPromotionService {
	svc := &EmailPromotionService{
		repo:                 repo,
		orderPushClaimer:     repo,
		messageCenterInitErr: messageCenterInitErr,
		messageCenterFactory: messagecenter.NewClientFromEnv,
		submissionSlots:      make(chan struct{}, promotionSubmissionWorkers),
	}
	if messageCenter != nil {
		svc.messageCenter = messageCenter
	}
	return svc
}

// TriggerPromotion validates ownership and creates a promotion, then submits it asynchronously.
func (s *EmailPromotionService) TriggerPromotion(ctx context.Context, userID, orderID, projectID int) (*TriggerPromotionResult, error) {
	return s.TriggerPromotionWithInput(ctx, userID, TriggerPromotionInput{
		OrderID:   orderID,
		ProjectID: projectID,
		Strategy:  "region",
	})
}

type TriggerPromotionInput struct {
	OrderID                 int
	ProjectID               int
	Strategy                string
	MaxRecipients           *int
	OrderPushAlreadyPending bool
}

// TriggerPromotionWithInput validates ownership and creates a promotion, then submits it asynchronously.
func (s *EmailPromotionService) TriggerPromotionWithInput(ctx context.Context, userID int, input TriggerPromotionInput) (*TriggerPromotionResult, error) {
	orderID := input.OrderID
	projectID := input.ProjectID
	strategy := normalizePromotionStrategy(input.Strategy)
	if !isValidPromotionStrategy(strategy) {
		return nil, ErrBadRequest("invalid promotion strategy")
	}

	order, err := s.repo.Order.GetByID(ctx, orderID)
	if err != nil {
		return nil, ErrInternal("获取订单失败")
	}
	if order == nil {
		return nil, ErrNotFound("订单不存在")
	}
	if order.UserID != userID {
		return nil, ErrForbidden("无权操作此订单")
	}
	if order.Status != models.OrderStatusPaid {
		return nil, ErrBadRequest("订单未支付或状态异常")
	}

	project, err := s.repo.Project.GetByID(ctx, projectID)
	if err != nil {
		return nil, ErrInternal("获取项目失败")
	}
	if project == nil {
		return nil, ErrNotFound("项目不存在")
	}
	if project.CreatorID != userID {
		return nil, ErrForbidden("只能推广自己创建的项目")
	}

	channel := "EMAIL"
	businessTag := "project_promotion"
	traceID := fmt.Sprintf("PROJECT_PROMOTION:%d", orderID)

	existingPromotion, err := s.repo.EmailPromotion.GetByOrderAndProject(ctx, orderID, projectID)
	if err != nil {
		return nil, ErrInternal("检查推广记录失败")
	}
	if existingPromotion != nil {
		shouldRedrive := existingPromotion.Status == models.EmailPromotionStatusPending ||
			existingPromotion.Status == models.EmailPromotionStatusFailed ||
			(input.OrderPushAlreadyPending && existingPromotion.Status == models.EmailPromotionStatusSending)
		if input.OrderPushAlreadyPending && existingPromotion.Status == models.EmailPromotionStatusCompleted {
			if err := s.completeOrderPush(ctx, orderID, userID); err != nil {
				return nil, err
			}
		}
		if !shouldRedrive {
			return &TriggerPromotionResult{
				Promotion:     existingPromotion,
				MaxRecipients: existingPromotion.MaxRecipients,
			}, nil
		}
		maxRecipients := existingPromotion.MaxRecipients
		if maxRecipients == 0 {
			maxRecipients = order.Quantity
		}
		if input.MaxRecipients != nil && *input.MaxRecipients != maxRecipients {
			return nil, ErrBadRequest("maxRecipients must equal paid order quantity")
		}
		if !input.OrderPushAlreadyPending {
			if err := s.markOrderPushPending(ctx, orderID, userID); err != nil {
				return nil, err
			}
		}
		if existingPromotion.Strategy == "" {
			existingPromotion.Strategy = strategy
		}
		existingPromotion.MaxRecipients = maxRecipients
		existingPromotion.Channel = &channel
		existingPromotion.BusinessTag = &businessTag
		existingPromotion.TraceID = &traceID
		existingPromotion.ProjectID = projectID
		existingPromotion.CreatorID = userID
		if updateErr := s.repo.EmailPromotion.UpdateMetadata(ctx, existingPromotion); updateErr != nil {
			log.Printf("[EmailPromotionService] failed to normalize existing email promotion, order_id=%d project_id=%d: %v", orderID, projectID, updateErr)
			return nil, ErrInternal("更新推广记录失败")
		}
		recipientUserIDs, selectErr := s.repo.EmailPromotion.GetRetryRecipientUserIDs(ctx, existingPromotion.ID, maxRecipients)
		if selectErr != nil {
			log.Printf("[EmailPromotionService] failed to load original promotion recipients: %v", selectErr)
			return nil, ErrInternal("load original promotion recipients failed")
		}
		if len(recipientUserIDs) == 0 {
			log.Printf("[EmailPromotionService] legacy promotion has no recipient history; selecting a compatibility fallback, promotion_id=%d", existingPromotion.ID)
			recipientUserIDs, selectErr = s.repo.EmailPromotion.SelectPromotionRecipients(ctx, projectID, userID, existingPromotion.Strategy, maxRecipients)
			if selectErr != nil {
				log.Printf("[EmailPromotionService] failed to select fallback recipients for existing promotion: %v", selectErr)
				return nil, ErrInternal("select promotion recipients failed")
			}
		}
		if createErr := s.repo.EmailPromotion.CreateRecipients(ctx, existingPromotion.ID, projectID, recipientUserIDs); createErr != nil {
			log.Printf("[EmailPromotionService] failed to create recipients for existing promotion: %v", createErr)
			return nil, ErrInternal("create promotion recipients failed")
		}
		req := messagecenter.ProjectPromotionRequest{
			PromotionID:      existingPromotion.ID,
			ProjectID:        project.ID,
			PromotionCount:   maxRecipients,
			CreatorUserID:    project.CreatorID,
			OrderID:          orderID,
			Strategy:         existingPromotion.Strategy,
			TraceID:          traceID,
			RecipientUserIDs: recipientUserIDs,
		}
		s.startAsyncPromotionSubmission(req)
		return &TriggerPromotionResult{
			Promotion:     existingPromotion,
			MaxRecipients: existingPromotion.MaxRecipients,
		}, nil
	}

	maxRecipients, err := s.calculateMaxRecipients(ctx, order)
	if err != nil {
		return nil, err
	}
	if input.MaxRecipients != nil && *input.MaxRecipients != maxRecipients {
		return nil, ErrBadRequest("maxRecipients must equal paid order quantity")
	}
	if !input.OrderPushAlreadyPending {
		if err := s.markOrderPushPending(ctx, orderID, userID); err != nil {
			return nil, err
		}
	}

	promotion := &models.EmailPromotion{
		Channel:       &channel,
		BusinessTag:   &businessTag,
		TraceID:       &traceID,
		OrderID:       orderID,
		ProjectID:     projectID,
		CreatorID:     userID,
		Strategy:      strategy,
		MaxRecipients: maxRecipients,
		Status:        models.EmailPromotionStatusPending,
	}

	if err := s.repo.EmailPromotion.Create(ctx, promotion); err != nil {
		log.Printf("[EmailPromotionService] failed to create email promotion: %v", err)
		return nil, ErrInternal("创建推广记录失败")
	}

	recipientUserIDs, err := s.repo.EmailPromotion.SelectPromotionRecipients(ctx, projectID, userID, strategy, maxRecipients)
	if err != nil {
		log.Printf("[EmailPromotionService] failed to select promotion recipients: %v", err)
		return nil, ErrInternal("select promotion recipients failed")
	}
	if err := s.repo.EmailPromotion.CreateRecipients(ctx, promotion.ID, projectID, recipientUserIDs); err != nil {
		log.Printf("[EmailPromotionService] failed to create promotion recipients: %v", err)
		return nil, ErrInternal("create promotion recipients failed")
	}
	req := messagecenter.ProjectPromotionRequest{
		PromotionID:      promotion.ID,
		ProjectID:        project.ID,
		PromotionCount:   maxRecipients,
		CreatorUserID:    project.CreatorID,
		OrderID:          orderID,
		Strategy:         strategy,
		TraceID:          traceID,
		RecipientUserIDs: recipientUserIDs,
	}
	s.startAsyncPromotionSubmission(req)

	return &TriggerPromotionResult{
		Promotion:     promotion,
		MaxRecipients: maxRecipients,
	}, nil
}

// TriggerPromotionResult holds the result of triggering a promotion.
type TriggerPromotionResult struct {
	Promotion     *models.EmailPromotion
	MaxRecipients int
}

func (s *EmailPromotionService) markOrderPushPending(ctx context.Context, orderID, userID int) error {
	claimer := s.orderPushClaimer
	if claimer == nil {
		claimer = s.repo
	}
	updated, err := claimer.BeginOrderPushDeliveryForUser(ctx, orderID, userID)
	if err != nil {
		return ErrInternal("update order push status failed")
	}
	if !updated {
		return ErrBadRequest("order delivery is already pending or completed")
	}
	return nil
}

func (s *EmailPromotionService) completeOrderPush(ctx context.Context, orderID, userID int) error {
	updated, err := s.repo.UpdateOrderPushStatusForUser(ctx, orderID, userID, "success", nil)
	if err != nil {
		return ErrInternal("update order push status failed")
	}
	if !updated {
		return ErrForbidden("no permission to update order push status")
	}
	return nil
}

func normalizePromotionStrategy(strategy string) string {
	strategy = strings.TrimSpace(strings.ToLower(strategy))
	if strategy == "" {
		return "region"
	}
	return strategy
}

func isValidPromotionStrategy(strategy string) bool {
	switch strategy {
	case "region", "project", "major":
		return true
	default:
		return false
	}
}

func (s *EmailPromotionService) calculateMaxRecipients(ctx context.Context, order *models.Order) (int, error) {
	product, err := s.repo.Product.GetByID(ctx, order.ProductID)
	if err != nil || product == nil {
		return 0, ErrBadRequest("无法获取商品信息")
	}

	if product.Type == models.ProductTypeBenefit {
		return order.Quantity, nil
	}

	return 0, ErrBadRequest("订单中没有邮件推广商品")
}

func (s *EmailPromotionService) startAsyncPromotionSubmission(req messagecenter.ProjectPromotionRequest) {
	req.RecipientUserIDs = append([]int(nil), req.RecipientUserIDs...)
	select {
	case s.submissionSlots <- struct{}{}:
		s.submissionWG.Add(1)
	default:
		s.markPromotionFailed(req, "promotion submission capacity is temporarily exhausted")
		return
	}
	go func() {
		defer func() {
			<-s.submissionSlots
			s.submissionWG.Done()
		}()
		submitter, initErr, baseURL := s.resolveMessageCenter()
		if initErr != nil {
			log.Printf("[EmailPromotionService] message center unavailable for promotion, order_id=%d project_id=%d base_url_empty=%t: %v",
				req.OrderID, req.ProjectID, baseURL == "", initErr)
			s.markPromotionFailed(req, "message center is not configured: "+initErr.Error())
			return
		}
		if submitter == nil {
			log.Printf("[EmailPromotionService] message center client nil for promotion, order_id=%d project_id=%d base_url_empty=%t",
				req.OrderID, req.ProjectID, baseURL == "")
			s.markPromotionFailed(req, "message center client is nil")
			return
		}
		log.Printf("[EmailPromotionService] submitting project promotion, promotion_id=%d order_id=%d project_id=%d base_url=%s",
			req.PromotionID, req.OrderID, req.ProjectID, baseURL)

		var (
			resp *messagecenter.ProjectPromotionResponse
			err  error
		)
		for attempt := 1; attempt <= 3; attempt++ {
			callCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			resp, err = submitter.SubmitProjectPromotion(callCtx, req)
			cancel()
			if err == nil {
				break
			}
			log.Printf("[EmailPromotionService] submit project promotion failed, order_id=%d project_id=%d attempt=%d: %v",
				req.OrderID, req.ProjectID, attempt, err)
			if attempt < 3 {
				time.Sleep(time.Duration(attempt) * 200 * time.Millisecond)
			}
		}
		if err != nil {
			s.markPromotionFailed(req, "submit message center failed: "+err.Error())
			return
		}

		log.Printf("[EmailPromotionService] submitted project promotion, promotion_id=%d order_id=%d project_id=%d task_id=%s requested=%d actual=%d",
			req.PromotionID, req.OrderID, req.ProjectID, resp.TaskID, resp.RequestedCount, resp.ActualCount)

		// Recipient user IDs are selected and snapshotted before submission so
		// the project owner can inspect the batch immediately.
	}()
}

// WaitForSubmissions lets process shutdown drain accepted promotion work.
func (s *EmailPromotionService) WaitForSubmissions(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		s.submissionWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *EmailPromotionService) resolveMessageCenter() (projectPromotionSubmitter, error, string) {
	s.mu.RLock()
	submitter := s.messageCenter
	initErr := s.messageCenterInitErr
	baseURL := messageCenterBaseURL(submitter)
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
		return s.messageCenter, s.messageCenterInitErr, messageCenterBaseURL(s.messageCenter)
	}
	s.messageCenter = client
	s.messageCenterInitErr = nil
	log.Printf("[EmailPromotionService] message center configured after lazy reload, base_url=%s", client.BaseURL())
	return s.messageCenter, nil, client.BaseURL()
}

func messageCenterBaseURL(submitter projectPromotionSubmitter) string {
	if client, ok := submitter.(*messagecenter.Client); ok {
		return client.BaseURL()
	}
	return ""
}

func (s *EmailPromotionService) markPromotionFailed(req messagecenter.ProjectPromotionRequest, message string) {
	log.Printf("[EmailPromotionService] project promotion submission failed, promotion_id=%d order_id=%d project_id=%d: %s",
		req.PromotionID, req.OrderID, req.ProjectID, message)
	now := time.Now()
	updated, err := s.repo.EmailPromotion.MarkFailedIfNotCompleted(context.Background(), req.PromotionID, message, now)
	if err != nil {
		log.Printf("[EmailPromotionService] conditionally fail promotion, promotion_id=%d order_id=%d: %v",
			req.PromotionID, req.OrderID, err)
		return
	}
	if !updated {
		return
	}
	_, _ = s.repo.UpdateOrderPushStatusForUser(context.Background(), req.OrderID, req.CreatorUserID, "failed", &message)
}

// GetStatus retrieves a promotion record with ownership check.
func (s *EmailPromotionService) GetStatus(ctx context.Context, userID, promotionID int) (*models.EmailPromotion, error) {
	promotion, err := s.repo.EmailPromotion.GetByID(ctx, promotionID)
	if err != nil {
		return nil, ErrInternal("获取推广记录失败")
	}
	if promotion == nil {
		return nil, ErrNotFound("推广记录不存在")
	}
	if promotion.CreatorID != userID {
		return nil, ErrForbidden("无权查看此推广记录")
	}
	return promotion, nil
}

// ListByCreator lists promotions created by a user.
func (s *EmailPromotionService) ListByCreator(ctx context.Context, userID, page, size int) ([]models.EmailPromotion, int64, error) {
	promotions, total, err := s.repo.EmailPromotion.ListByCreatorID(ctx, userID, page, size)
	if err != nil {
		return nil, 0, ErrInternal("获取推广记录失败")
	}
	return promotions, total, nil
}

// ProjectPromotionBatchVO is the project dashboard batch row.
type ProjectPromotionBatchVO struct {
	ID            int                         `json:"id"`
	BatchID       int                         `json:"batchId"`
	OrderID       int                         `json:"orderId"`
	ProjectID     int                         `json:"projectId"`
	Strategy      string                      `json:"strategy"`
	MaxRecipients int                         `json:"maxRecipients"`
	TotalSent     int                         `json:"totalSent"`
	SuccessCount  int                         `json:"successCount"`
	Status        models.EmailPromotionStatus `json:"status"`
	StatusText    string                      `json:"statusText"`
	PromotedAt    time.Time                   `json:"promotedAt"`
	CreatedAt     time.Time                   `json:"createdAt"`
	StartedAt     *time.Time                  `json:"startedAt,omitempty"`
	CompletedAt   *time.Time                  `json:"completedAt,omitempty"`
	Channel       string                      `json:"channel"`
	BusinessTag   string                      `json:"businessTag"`
	TraceID       string                      `json:"traceId"`
}

// ProjectPromotionBatchListResult holds recent project promotion batches.
type ProjectPromotionBatchListResult struct {
	Total int64                     `json:"total"`
	List  []ProjectPromotionBatchVO `json:"list"`
}

// ProjectPromotionUserListResult holds users reached by a promotion batch.
type ProjectPromotionUserListResult struct {
	Total int64                             `json:"total"`
	List  []repository.ProjectPromotionUser `json:"list"`
}

// ListProjectBatches returns recent promotion batches for a project owner.
func (s *EmailPromotionService) ListProjectBatches(ctx context.Context, requesterUserID, projectID, days, limit int) (*ProjectPromotionBatchListResult, error) {
	if days <= 0 {
		days = 7
	}
	if days > 30 {
		days = 30
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	isReviewer, err := s.repo.Project.IsOwnerOrMember(ctx, projectID, requesterUserID)
	if err != nil {
		log.Printf("[EmailPromotionService.ListProjectBatches] ownership check error: %v", err)
		return nil, ErrInternal("check project permission failed")
	}
	if !isReviewer {
		return nil, ErrForbidden("no permission to view project promotion records")
	}

	promotions, total, err := s.repo.EmailPromotion.ListByProjectSince(ctx, projectID, days, limit)
	if err != nil {
		log.Printf("[EmailPromotionService.ListProjectBatches] query error: %v", err)
		return nil, ErrInternal("list project promotion batches failed")
	}

	list := make([]ProjectPromotionBatchVO, len(promotions))
	for i, p := range promotions {
		promotedAt := p.CreatedAt
		if p.StartedAt != nil {
			promotedAt = *p.StartedAt
		}
		list[i] = ProjectPromotionBatchVO{
			ID:            p.ID,
			BatchID:       p.ID,
			OrderID:       p.OrderID,
			ProjectID:     p.ProjectID,
			Strategy:      p.Strategy,
			MaxRecipients: p.MaxRecipients,
			TotalSent:     p.TotalSent,
			SuccessCount:  p.TotalSent,
			Status:        p.Status,
			StatusText:    emailPromotionStatusText(p.Status),
			PromotedAt:    promotedAt,
			CreatedAt:     p.CreatedAt,
			StartedAt:     p.StartedAt,
			CompletedAt:   p.CompletedAt,
			Channel:       stringValue(p.Channel),
			BusinessTag:   stringValue(p.BusinessTag),
			TraceID:       stringValue(p.TraceID),
		}
	}

	return &ProjectPromotionBatchListResult{Total: total, List: list}, nil
}

// ListProjectBatchesPaged returns paged promotion batches for a project owner.
func (s *EmailPromotionService) ListProjectBatchesPaged(ctx context.Context, requesterUserID, projectID, page, size, days, limit int) (*ProjectPromotionBatchListResult, error) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 10
	}
	if size > 50 {
		size = 50
	}
	if days < 0 {
		days = 0
	}
	if days > 30 {
		days = 30
	}
	if limit < 0 {
		limit = 0
	}
	if limit > 50 {
		limit = 50
	}

	isReviewer, err := s.repo.Project.IsOwnerOrMember(ctx, projectID, requesterUserID)
	if err != nil {
		log.Printf("[EmailPromotionService.ListProjectBatchesPaged] ownership check error: %v", err)
		return nil, ErrInternal("check project permission failed")
	}
	if !isReviewer {
		return nil, ErrForbidden("no permission to view project promotion records")
	}

	promotions, total, err := s.repo.EmailPromotion.ListByProjectPaged(ctx, projectID, page, size, days, limit)
	if err != nil {
		log.Printf("[EmailPromotionService.ListProjectBatchesPaged] query error: %v", err)
		return nil, ErrInternal("list project promotion batches failed")
	}

	list := make([]ProjectPromotionBatchVO, len(promotions))
	for i, p := range promotions {
		promotedAt := p.CreatedAt
		if p.StartedAt != nil {
			promotedAt = *p.StartedAt
		}
		list[i] = ProjectPromotionBatchVO{
			ID:            p.ID,
			BatchID:       p.ID,
			OrderID:       p.OrderID,
			ProjectID:     p.ProjectID,
			Strategy:      p.Strategy,
			MaxRecipients: p.MaxRecipients,
			TotalSent:     p.TotalSent,
			SuccessCount:  p.TotalSent,
			Status:        p.Status,
			StatusText:    emailPromotionStatusText(p.Status),
			PromotedAt:    promotedAt,
			CreatedAt:     p.CreatedAt,
			StartedAt:     p.StartedAt,
			CompletedAt:   p.CompletedAt,
			Channel:       stringValue(p.Channel),
			BusinessTag:   stringValue(p.BusinessTag),
			TraceID:       stringValue(p.TraceID),
		}
	}

	return &ProjectPromotionBatchListResult{Total: total, List: list}, nil
}

// ListProjectBatchUsers returns safe recipient user rows for a project batch.
func (s *EmailPromotionService) ListProjectBatchUsers(ctx context.Context, requesterUserID, projectID, batchID, page, size int) (*ProjectPromotionUserListResult, error) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	if size > 100 {
		size = 100
	}

	isReviewer, err := s.repo.Project.IsOwnerOrMember(ctx, projectID, requesterUserID)
	if err != nil {
		log.Printf("[EmailPromotionService.ListProjectBatchUsers] ownership check error: %v", err)
		return nil, ErrInternal("check project permission failed")
	}
	if !isReviewer {
		return nil, ErrForbidden("no permission to view project promotion records")
	}

	promotion, err := s.repo.EmailPromotion.GetByID(ctx, batchID)
	if err != nil {
		log.Printf("[EmailPromotionService.ListProjectBatchUsers] get promotion error: %v", err)
		return nil, ErrInternal("get promotion batch failed")
	}
	if promotion == nil || promotion.ProjectID != projectID {
		return nil, ErrNotFound("promotion batch not found")
	}

	users, total, err := s.repo.EmailPromotion.ListProjectPromotionUsers(ctx, batchID, page, size)
	if err != nil {
		log.Printf("[EmailPromotionService.ListProjectBatchUsers] query users error: %v", err)
		return nil, ErrInternal("list promotion batch users failed")
	}

	return &ProjectPromotionUserListResult{Total: total, List: users}, nil
}

func emailPromotionStatusText(status models.EmailPromotionStatus) string {
	switch status {
	case models.EmailPromotionStatusPending:
		return "\u5f85\u53d1\u9001"
	case models.EmailPromotionStatusSending:
		return "\u53d1\u9001\u4e2d"
	case models.EmailPromotionStatusCompleted:
		return "\u5df2\u5b8c\u6210"
	case models.EmailPromotionStatusFailed:
		return "\u53d1\u9001\u5931\u8d25"
	default:
		return "\u672a\u77e5\u72b6\u6001"
	}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
