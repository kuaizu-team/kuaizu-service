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

const (
	paidOrderDeliveryTimeout       = 30 * time.Second
	paidOrderDeliveryStaleAfter    = 5 * time.Minute
	paidOrderDeliveryRecoveryEvery = 30 * time.Second
	paidOrderDeliveryRecoveryLimit = 100
	paidOrderDeliveryWorkers       = 8
)

type paidOrderDeliverer interface {
	Deliver(ctx context.Context, order *models.Order, orderPushAlreadyPending bool) error
}

type orderDeliveryStateRepository interface {
	BeginOrderPushDeliveryForUser(ctx context.Context, id, userID int) (bool, error)
	ListRecoverableOrderDeliveries(ctx context.Context, staleBefore time.Time, limit int) ([]*models.Order, error)
	ClaimRecoverableOrderDelivery(ctx context.Context, id int, staleBefore time.Time) (bool, error)
	UpdateOrderPushStatus(ctx context.Context, id int, status string, errorMessage *string) error
}

// PaymentService handles payment-related business logic.
type PaymentService struct {
	repo          *repository.Repository
	payClient     *wechat.PayClient
	payInitErr    error
	delivery      paidOrderDeliverer
	deliveryState orderDeliveryStateRepository
	recoverySlots chan struct{}
}

func (s *PaymentService) SetPaidOrderDeliveryService(delivery paidOrderDeliverer) {
	s.delivery = delivery
}

// EnsurePaidOrderDelivery durably claims delivery after payment commits and detaches it from the webhook context.
func (s *PaymentService) EnsurePaidOrderDelivery(_ context.Context, order *models.Order) {
	if s.delivery == nil || order == nil {
		return
	}
	intent, err := order.ParseDeliveryIntent()
	if err != nil {
		s.markOrderDeliveryFailed(order.ID, err)
		return
	}
	if intent == nil {
		return
	}
	claimCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	claimed, claimErr := s.deliveryState.BeginOrderPushDeliveryForUser(claimCtx, order.ID, order.UserID)
	cancel()
	if claimErr != nil {
		log.Printf("[PaymentService.EnsurePaidOrderDelivery] claim failed, order_id=%d: %v", order.ID, claimErr)
		return
	}
	if claimed {
		go s.deliverClaimedOrder(order)
	}
}

// StartOrderDeliveryRecovery recovers paid orders left unclaimed or stale after an interruption.
func (s *PaymentService) StartOrderDeliveryRecovery(ctx context.Context) {
	go func() {
		s.recoverOrderDeliveries(ctx)
		ticker := time.NewTicker(paidOrderDeliveryRecoveryEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.recoverOrderDeliveries(ctx)
			}
		}
	}()
}

func (s *PaymentService) recoverOrderDeliveries(ctx context.Context) {
	queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	staleBefore := time.Now().Add(-paidOrderDeliveryStaleAfter)
	orders, err := s.deliveryState.ListRecoverableOrderDeliveries(queryCtx, staleBefore, paidOrderDeliveryRecoveryLimit)
	if err != nil {
		log.Printf("[PaymentService] list recoverable order deliveries failed: %v", err)
		return
	}
	for _, order := range orders {
		select {
		case s.recoverySlots <- struct{}{}:
		case <-ctx.Done():
			return
		}
		go func(candidate *models.Order) {
			defer func() { <-s.recoverySlots }()
			claimCtx, claimCancel := context.WithTimeout(context.Background(), 5*time.Second)
			claimed, claimErr := s.deliveryState.ClaimRecoverableOrderDelivery(claimCtx, candidate.ID, staleBefore)
			claimCancel()
			if claimErr != nil {
				log.Printf("[PaymentService] claim recoverable order_id=%d failed: %v", candidate.ID, claimErr)
				return
			}
			if claimed {
				s.deliverClaimedOrder(candidate)
			}
		}(order)
	}
}

func (s *PaymentService) deliverClaimedOrder(order *models.Order) {
	deliveryCtx, cancel := context.WithTimeout(context.Background(), paidOrderDeliveryTimeout)
	err := s.delivery.Deliver(deliveryCtx, order, true)
	cancel()
	if err != nil {
		s.markOrderDeliveryFailed(order.ID, err)
	}
}

func (s *PaymentService) markOrderDeliveryFailed(orderID int, deliveryErr error) {
	message := deliveryErr.Error()
	updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.deliveryState.UpdateOrderPushStatus(updateCtx, orderID, "failed", &message); err != nil {
		log.Printf("[PaymentService] delivery failed and state update failed, order_id=%d delivery_error=%v update_error=%v", orderID, deliveryErr, err)
		return
	}
	log.Printf("[PaymentService] delivery failed, order_id=%d: %v", orderID, deliveryErr)
}

// NewPaymentService creates a new PaymentService.
func NewPaymentService(repo *repository.Repository, payClient *wechat.PayClient, payInitErr error) *PaymentService {
	return &PaymentService{
		repo:          repo,
		payClient:     payClient,
		payInitErr:    payInitErr,
		deliveryState: repo,
		recoverySlots: make(chan struct{}, paidOrderDeliveryWorkers),
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

func canProcessPaymentStatus(status int, acceptCancelled bool) bool {
	return status == models.OrderStatusPending || (acceptCancelled && status == models.OrderStatusCancelled)
}

// ProcessPayment updates an ordinary pending order and distributes benefits within a DB transaction.
func (s *PaymentService) ProcessPayment(ctx context.Context, order *models.Order, transactionID string, payTime time.Time) error {
	return s.processPayment(ctx, order, transactionID, payTime, false)
}

// ProcessVirtualPayment also accepts a locally cancelled order because the
// signed WeChat delivery notification is authoritative evidence that payment
// completed before a client-side cancel/timeout race. Ordinary payment flows
// remain pending-only.
func (s *PaymentService) ProcessVirtualPayment(ctx context.Context, order *models.Order, transactionID string, payTime time.Time) error {
	return s.processPayment(ctx, order, transactionID, payTime, true)
}

func (s *PaymentService) processPayment(ctx context.Context, order *models.Order, transactionID string, payTime time.Time, acceptCancelled bool) error {
	tx, err := s.repo.DB().BeginTxx(ctx, nil)
	if err != nil {
		log.Printf("[PaymentService.ProcessPayment] failed to begin transaction: %v", err)
		return ErrInternal("处理支付失败")
	}
	defer tx.Rollback()

	// Lock the order before granting benefits. WeChat may retry callbacks, and
	// virtual-payment delivery notifications are explicitly retried by the platform.
	var currentStatus int
	if err := tx.GetContext(ctx, &currentStatus, "SELECT status FROM `order` WHERE id=? FOR UPDATE", order.ID); err != nil {
		log.Printf("[PaymentService.ProcessPayment] failed to lock order: %v", err)
		return ErrInternal("处理支付失败")
	}
	if currentStatus == models.OrderStatusPaid {
		return nil
	}
	if !canProcessPaymentStatus(currentStatus, acceptCancelled) {
		return ErrBadRequest("订单状态不允许支付")
	}

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
		totalBranches := product.OliveBranchCount() * order.Quantity
		if err := s.repo.User.AddOliveBranchCountTx(ctx, tx, order.UserID, totalBranches); err != nil {
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
