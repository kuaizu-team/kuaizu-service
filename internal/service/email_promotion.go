package service

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/messagecenter"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

// EmailPromotionService handles email promotion business logic.
type EmailPromotionService struct {
	repo                 *repository.Repository
	mu                   sync.RWMutex
	messageCenter        projectPromotionSubmitter
	messageCenterInitErr error
	messageCenterFactory func() (*messagecenter.Client, error)
}

type projectPromotionSubmitter interface {
	SubmitProjectPromotion(ctx context.Context, req messagecenter.ProjectPromotionRequest) (*messagecenter.ProjectPromotionResponse, error)
}

// NewEmailPromotionService creates a new EmailPromotionService.
func NewEmailPromotionService(repo *repository.Repository) *EmailPromotionService {
	return &EmailPromotionService{
		repo:                 repo,
		messageCenterFactory: messagecenter.NewClientFromEnv,
	}
}

// NewEmailPromotionServiceWithMessageCenter creates a service with a message-center client.
func NewEmailPromotionServiceWithMessageCenter(repo *repository.Repository, messageCenter *messagecenter.Client, messageCenterInitErr error) *EmailPromotionService {
	svc := &EmailPromotionService{
		repo:                 repo,
		messageCenterInitErr: messageCenterInitErr,
		messageCenterFactory: messagecenter.NewClientFromEnv,
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
	OrderID       int
	ProjectID     int
	Strategy      string
	MaxRecipients *int
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

	existingPromotion, err := s.repo.EmailPromotion.GetByOrderID(ctx, orderID)
	if err != nil {
		return nil, ErrInternal("检查推广记录失败")
	}
	if existingPromotion != nil {
		return nil, ErrBadRequest("此订单已触发过推广")
	}

	maxRecipients, err := s.calculateMaxRecipients(ctx, order)
	if err != nil {
		return nil, err
	}
	if input.MaxRecipients != nil && *input.MaxRecipients != maxRecipients {
		return nil, ErrBadRequest("maxRecipients must equal paid order quantity")
	}

	promotion := &models.EmailPromotion{
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
		ProjectID:        project.ID,
		PromotionCount:   maxRecipients,
		CreatorUserID:    project.CreatorID,
		OrderID:          orderID,
		Strategy:         strategy,
		RecipientUserIDs: recipientUserIDs,
	}
	s.startAsyncPromotionSubmission(promotion, req)

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

func (s *EmailPromotionService) startAsyncPromotionSubmission(promotion *models.EmailPromotion, req messagecenter.ProjectPromotionRequest) {
	go func() {
		submitter, initErr, baseURL := s.resolveMessageCenter()
		if initErr != nil {
			log.Printf("[EmailPromotionService] message center unavailable for promotion, order_id=%d project_id=%d base_url_empty=%t: %v",
				req.OrderID, req.ProjectID, baseURL == "", initErr)
			s.markPromotionFailed(promotion, "message center is not configured: "+initErr.Error())
			return
		}
		if submitter == nil {
			log.Printf("[EmailPromotionService] message center client nil for promotion, order_id=%d project_id=%d base_url_empty=%t",
				req.OrderID, req.ProjectID, baseURL == "")
			s.markPromotionFailed(promotion, "message center client is nil")
			return
		}
		log.Printf("[EmailPromotionService] submitting project promotion, promotion_id=%d order_id=%d project_id=%d base_url=%s",
			promotion.ID, promotion.OrderID, promotion.ProjectID, baseURL)

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
			s.markPromotionFailed(promotion, "submit message center failed: "+err.Error())
			return
		}

		now := time.Now()
		promotion.Status = models.EmailPromotionStatusSending
		promotion.TotalSent = resp.ActualCount
		promotion.StartedAt = &now
		promotion.ErrorMessage = nil
		if updateErr := s.repo.EmailPromotion.Update(context.Background(), promotion); updateErr != nil {
			log.Printf("[EmailPromotionService] failed to update submitted promotion, promotion_id=%d order_id=%d task_id=%s: %v",
				promotion.ID, promotion.OrderID, resp.TaskID, updateErr)
		}
		log.Printf("[EmailPromotionService] submitted project promotion, promotion_id=%d order_id=%d project_id=%d task_id=%s requested=%d actual=%d",
			promotion.ID, promotion.OrderID, promotion.ProjectID, resp.TaskID, resp.RequestedCount, resp.ActualCount)

		// Recipient user IDs are determined by the message center for this path.
		// email_promotion_recipient should be written by the message-center
		// callback/sync that reports the concrete email_task and user results.
	}()
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

func (s *EmailPromotionService) markPromotionFailed(promotion *models.EmailPromotion, message string) {
	log.Printf("[EmailPromotionService] project promotion submission failed, promotion_id=%d order_id=%d project_id=%d: %s",
		promotion.ID, promotion.OrderID, promotion.ProjectID, message)
	promotion.Status = models.EmailPromotionStatusFailed
	promotion.ErrorMessage = &message
	if err := s.repo.EmailPromotion.Update(context.Background(), promotion); err != nil {
		log.Printf("[EmailPromotionService] failed to update failed promotion, promotion_id=%d order_id=%d: %v",
			promotion.ID, promotion.OrderID, err)
	}
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
	ProjectID     int                         `json:"projectId"`
	Strategy      string                      `json:"strategy"`
	MaxRecipients int                         `json:"maxRecipients"`
	TotalSent     int                         `json:"totalSent"`
	Status        models.EmailPromotionStatus `json:"status"`
	StatusText    string                      `json:"statusText"`
	PromotedAt    time.Time                   `json:"promotedAt"`
	CreatedAt     time.Time                   `json:"createdAt"`
	StartedAt     *time.Time                  `json:"startedAt,omitempty"`
	CompletedAt   *time.Time                  `json:"completedAt,omitempty"`
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

	isOwner, err := s.repo.Project.IsOwner(ctx, projectID, requesterUserID)
	if err != nil {
		log.Printf("[EmailPromotionService.ListProjectBatches] ownership check error: %v", err)
		return nil, ErrInternal("check project permission failed")
	}
	if !isOwner {
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
		if p.CompletedAt != nil {
			promotedAt = *p.CompletedAt
		}
		if p.StartedAt != nil {
			promotedAt = *p.StartedAt
		}
		list[i] = ProjectPromotionBatchVO{
			ID:            p.ID,
			BatchID:       p.ID,
			ProjectID:     p.ProjectID,
			Strategy:      p.Strategy,
			MaxRecipients: p.MaxRecipients,
			TotalSent:     p.TotalSent,
			Status:        p.Status,
			StatusText:    emailPromotionStatusText(p.Status),
			PromotedAt:    promotedAt,
			CreatedAt:     p.CreatedAt,
			StartedAt:     p.StartedAt,
			CompletedAt:   p.CompletedAt,
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

	isOwner, err := s.repo.Project.IsOwner(ctx, projectID, requesterUserID)
	if err != nil {
		log.Printf("[EmailPromotionService.ListProjectBatchesPaged] ownership check error: %v", err)
		return nil, ErrInternal("check project permission failed")
	}
	if !isOwner {
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
		if p.CompletedAt != nil {
			promotedAt = *p.CompletedAt
		}
		if p.StartedAt != nil {
			promotedAt = *p.StartedAt
		}
		list[i] = ProjectPromotionBatchVO{
			ID:            p.ID,
			BatchID:       p.ID,
			ProjectID:     p.ProjectID,
			Strategy:      p.Strategy,
			MaxRecipients: p.MaxRecipients,
			TotalSent:     p.TotalSent,
			Status:        p.Status,
			StatusText:    emailPromotionStatusText(p.Status),
			PromotedAt:    promotedAt,
			CreatedAt:     p.CreatedAt,
			StartedAt:     p.StartedAt,
			CompletedAt:   p.CompletedAt,
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

	isOwner, err := s.repo.Project.IsOwner(ctx, projectID, requesterUserID)
	if err != nil {
		log.Printf("[EmailPromotionService.ListProjectBatchUsers] ownership check error: %v", err)
		return nil, ErrInternal("check project permission failed")
	}
	if !isOwner {
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
