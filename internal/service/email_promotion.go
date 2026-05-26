package service

import (
	"context"
	"log"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/messagecenter"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

// EmailPromotionService handles email promotion business logic.
type EmailPromotionService struct {
	repo                 *repository.Repository
	messageCenter        projectPromotionSubmitter
	messageCenterInitErr error
}

type projectPromotionSubmitter interface {
	SubmitProjectPromotion(ctx context.Context, req messagecenter.ProjectPromotionRequest) (*messagecenter.ProjectPromotionResponse, error)
}

// NewEmailPromotionService creates a new EmailPromotionService.
func NewEmailPromotionService(repo *repository.Repository) *EmailPromotionService {
	return &EmailPromotionService{repo: repo}
}

// NewEmailPromotionServiceWithMessageCenter creates a service with a message-center client.
func NewEmailPromotionServiceWithMessageCenter(repo *repository.Repository, messageCenter *messagecenter.Client, messageCenterInitErr error) *EmailPromotionService {
	return &EmailPromotionService{
		repo:                 repo,
		messageCenter:        messageCenter,
		messageCenterInitErr: messageCenterInitErr,
	}
}

// TriggerPromotion validates ownership and creates a promotion, then submits it asynchronously.
func (s *EmailPromotionService) TriggerPromotion(ctx context.Context, userID, orderID, projectID int) (*TriggerPromotionResult, error) {
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

	promotion := &models.EmailPromotion{
		OrderID:       orderID,
		ProjectID:     projectID,
		CreatorID:     userID,
		MaxRecipients: maxRecipients,
		Status:        models.EmailPromotionStatusPending,
	}

	if err := s.repo.EmailPromotion.Create(ctx, promotion); err != nil {
		log.Printf("[EmailPromotionService] failed to create email promotion: %v", err)
		return nil, ErrInternal("创建推广记录失败")
	}

	req := messagecenter.ProjectPromotionRequest{
		ProjectID:          project.ID,
		PromotionCount:     maxRecipients,
		ProjectTitle:       project.Name,
		ProjectDescription: truncateRunes(derefString(project.Description), 1000),
		CreatorUserID:      project.CreatorID,
		OrderID:            orderID,
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
		if s.messageCenterInitErr != nil {
			s.markPromotionFailed(promotion, "message center is not configured: "+s.messageCenterInitErr.Error())
			return
		}
		if s.messageCenter == nil {
			s.markPromotionFailed(promotion, "message center client is nil")
			return
		}

		var (
			resp *messagecenter.ProjectPromotionResponse
			err  error
		)
		for attempt := 1; attempt <= 3; attempt++ {
			callCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			resp, err = s.messageCenter.SubmitProjectPromotion(callCtx, req)
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
	}()
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

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
