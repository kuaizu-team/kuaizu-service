package service

import (
	"context"
	"log"

	"github.com/trv3wood/kuaizu-server/internal/email"
	"github.com/trv3wood/kuaizu-server/internal/models"
	"github.com/trv3wood/kuaizu-server/internal/repository"
)

// EmailPromotionService handles email promotion business logic.
type EmailPromotionService struct {
	repo         *repository.Repository
	emailService promotionEmailSender
	emailInitErr error
}

type promotionEmailSender interface {
	SendPromotionEmails(ctx context.Context, promotion *models.EmailPromotion)
}

// NewEmailPromotionService creates a new EmailPromotionService.
func NewEmailPromotionService(repo *repository.Repository) *EmailPromotionService {
	return &EmailPromotionService{repo: repo}
}

// NewEmailPromotionServiceWithEmail creates a new EmailPromotionService with an injected email sender.
func NewEmailPromotionServiceWithEmail(repo *repository.Repository, emailService *email.Service, emailInitErr error) *EmailPromotionService {
	return &EmailPromotionService{
		repo:         repo,
		emailService: emailService,
		emailInitErr: emailInitErr,
	}
}

// TriggerPromotionResult holds the result of triggering a promotion.
type TriggerPromotionResult struct {
	Promotion     *models.EmailPromotion
	MaxRecipients int
}

// TriggerPromotion validates ownership and creates a promotion, then starts async sending.
func (s *EmailPromotionService) TriggerPromotion(ctx context.Context, userID, orderID, projectID int) (*TriggerPromotionResult, error) {
	// Validate order ownership
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

	// Validate project ownership
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

	// Check duplication
	existingPromotion, err := s.repo.EmailPromotion.GetByOrderID(ctx, orderID)
	if err != nil {
		return nil, ErrInternal("检查推广记录失败")
	}
	if existingPromotion != nil {
		return nil, ErrBadRequest("此订单已触发过推广")
	}

	// Calculate max recipients from order items
	maxRecipients, err := s.calculateMaxRecipients(ctx, order)
	if err != nil {
		return nil, err
	}

	// Create promotion record
	promotion := &models.EmailPromotion{
		OrderID:       orderID,
		ProjectID:     projectID,
		CreatorID:     userID,
		MaxRecipients: maxRecipients,
		Status:        models.EmailPromotionStatusPending,
	}

	if err := s.repo.EmailPromotion.Create(ctx, promotion); err != nil {
		log.Printf("Failed to create email promotion: %v", err)
		return nil, ErrInternal("创建推广记录失败")
	}

	// Start async email sending
	s.startAsyncEmailSending(promotion)

	return &TriggerPromotionResult{
		Promotion:     promotion,
		MaxRecipients: maxRecipients,
	}, nil
}

func (s *EmailPromotionService) calculateMaxRecipients(ctx context.Context, order *models.Order) (int, error) {
	product, err := s.repo.Product.GetByID(ctx, order.ProductID)
	if err != nil || product == nil {
		return 0, ErrBadRequest("无法获取商品信息")
	}

	if product.Type == models.ProductTypeBenefit { // 服务权益 - 邮件推广
		return order.Quantity, nil
	}

	return 0, ErrBadRequest("订单中没有邮件推广商品")
}

func (s *EmailPromotionService) startAsyncEmailSending(promotion *models.EmailPromotion) {
	go func() {
		if s.emailInitErr != nil {
			errMsg := "邮件服务未配置: " + s.emailInitErr.Error()
			promotion.Status = models.EmailPromotionStatusFailed
			promotion.ErrorMessage = &errMsg
			s.repo.EmailPromotion.Update(context.Background(), promotion)
			return
		}

		if s.emailService == nil {
			errMsg := "邮件服务未配置"
			promotion.Status = models.EmailPromotionStatusFailed
			promotion.ErrorMessage = &errMsg
			s.repo.EmailPromotion.Update(context.Background(), promotion)
			return
		}

		s.emailService.SendPromotionEmails(context.Background(), promotion)
	}()
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
