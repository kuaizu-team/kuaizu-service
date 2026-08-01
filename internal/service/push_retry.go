package service

import (
	"context"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

// PushRetryService retries delivery while preserving the original paid order.
type PushRetryService struct {
	repo     *repository.Repository
	delivery *PaidOrderDeliveryService
}

func NewPushRetryService(repo *repository.Repository, email *EmailPromotionService, sms *SmsNoticeService) *PushRetryService {
	return &PushRetryService{repo: repo, delivery: NewPaidOrderDeliveryService(repo, email, sms)}
}

func (s *PushRetryService) Retry(ctx context.Context, userID, orderID int) (*models.Order, error) {
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
	if order.Status != models.OrderStatusPaid || order.RefundStatus != 0 {
		return nil, ErrBadRequest("订单状态不允许重试发送")
	}
	started, err := s.repo.BeginOrderPushRetry(ctx, orderID)
	if err != nil {
		return nil, ErrInternal("开始重试失败")
	}
	if !started {
		return nil, ErrBadRequest("仅发送失败的订单可以重试")
	}

	err = s.delivery.Deliver(ctx, order, true)
	if err != nil {
		message := err.Error()
		_, _ = s.fail(ctx, orderID, userID, message)
		return nil, err
	}
	updated, err := s.repo.Order.GetByID(ctx, orderID)
	if err != nil {
		return nil, ErrInternal("获取重试后的订单失败")
	}
	return updated, nil
}

func (s *PushRetryService) fail(ctx context.Context, orderID, userID int, message string) (*models.Order, error) {
	_, _ = s.repo.UpdateOrderPushStatusForUser(ctx, orderID, userID, "failed", &message)
	return nil, ErrInternal(message)
}
