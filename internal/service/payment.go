package service

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/wechat"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments"
)

// PaymentService handles payment-related business logic.
type PaymentService struct {
	repo       *repository.Repository
	payClient  *wechat.PayClient
	payInitErr error
}

// NewPaymentService creates a new PaymentService.
func NewPaymentService(repo *repository.Repository, payClient *wechat.PayClient, payInitErr error) *PaymentService {
	return &PaymentService{
		repo:       repo,
		payClient:  payClient,
		payInitErr: payInitErr,
	}
}

// ParseNotification parses and verifies a WeChat Pay callback.
func (s *PaymentService) ParseNotification(ctx context.Context, request *http.Request) (*payments.Transaction, error) {
	if s.payInitErr != nil {
		log.Printf("[PaymentService.ParseNotification] wechat pay init error: %v", s.payInitErr)
		return nil, ErrInternal("支付配置错误")
	}
	if s.payClient == nil {
		log.Printf("[PaymentService.ParseNotification] pay client is nil")
		return nil, ErrInternal("支付配置错误")
	}

	transaction, err := s.payClient.ParseNotification(ctx, request)
	if err != nil {
		log.Printf("[PaymentService.ParseNotification] parse notify error: %v", err)
		return nil, ErrBadRequest("验签失败")
	}

	return transaction, nil
}

// GetOrder retrieves an order by ID (returns nil, nil if not found).
func (s *PaymentService) GetOrder(ctx context.Context, orderID int) (*models.Order, error) {
	order, err := s.repo.Order.GetByID(ctx, orderID)
	if err != nil {
		log.Printf("[PaymentService.GetOrder] repository error: %v", err)
		return nil, ErrInternal("查询订单失败")
	}
	return order, nil
}

// MarkPaymentFailed updates order status to failed.
func (s *PaymentService) MarkPaymentFailed(ctx context.Context, orderID int) {
	s.repo.Order.UpdatePaymentStatus(ctx, orderID, 2, "", time.Now())
}

// ProcessPayment updates order status and distributes benefits within a DB transaction.
func (s *PaymentService) ProcessPayment(ctx context.Context, order *models.Order, transactionID string, payTime time.Time) error {
	tx, err := s.repo.DB().BeginTxx(ctx, nil)
	if err != nil {
		log.Printf("[PaymentService.ProcessPayment] failed to begin transaction: %v", err)
		return ErrInternal("处理支付失败")
	}
	defer tx.Rollback()

	// Update order status
	if err := s.repo.Order.UpdatePaymentStatusTx(ctx, tx, order.ID, 1, transactionID, payTime); err != nil {
		log.Printf("[PaymentService.ProcessPayment] failed to update order status: %v", err)
		return ErrInternal("处理支付失败")
	}

	// Distribute benefits
	product, err := s.repo.Product.GetByID(ctx, order.ProductID)
	if err != nil || product == nil {
		log.Printf("[PaymentService.ProcessPayment] failed to get product: %v", err)
		return ErrInternal("处理支付失败")
	}

	switch product.Type {
	case 1: // 橄榄枝
		if err := s.repo.User.AddOliveBranchCountTx(ctx, tx, order.UserID, order.Quantity); err != nil {
			log.Printf("[PaymentService.ProcessPayment] failed to add olive branch count: %v", err)
			return ErrInternal("处理支付失败")
		}
	case 2:
		// 权益需要凭订单和参数手动兑换
	default:
		log.Printf("[PaymentService.ProcessPayment] unknown product type: %d", product.Type)
	}

	if err := tx.Commit(); err != nil {
		log.Printf("[PaymentService.ProcessPayment] failed to commit transaction: %v", err)
		return ErrInternal("处理支付失败")
	}

	return nil
}
