package service

import (
	"context"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

// PaidOrderDeliveryService executes the intent captured when an order was created.
type PaidOrderDeliveryService struct {
	repo  *repository.Repository
	email *EmailPromotionService
	sms   *SmsNoticeService
}

func NewPaidOrderDeliveryService(repo *repository.Repository, email *EmailPromotionService, sms *SmsNoticeService) *PaidOrderDeliveryService {
	return &PaidOrderDeliveryService{repo: repo, email: email, sms: sms}
}

// Deliver is idempotent through the existing order-bound email and SMS records.
func (s *PaidOrderDeliveryService) Deliver(ctx context.Context, order *models.Order, orderPushAlreadyPending bool) error {
	intent, err := order.ParseDeliveryIntent()
	if err != nil {
		return ErrInternal("订单交付信息无效")
	}
	if intent == nil {
		if !orderPushAlreadyPending {
			return nil
		}
		return s.retryLegacyOrder(ctx, order)
	}

	switch intent.Scene {
	case models.OrderDeliverySceneEmailPromotion:
		if intent.ProjectID == nil || s.email == nil {
			return ErrInternal("邮件推广交付信息不完整")
		}
		_, err = s.email.TriggerPromotionWithInput(ctx, order.UserID, TriggerPromotionInput{
			OrderID:                 order.ID,
			ProjectID:               *intent.ProjectID,
			Strategy:                intent.Strategy,
			OrderPushAlreadyPending: orderPushAlreadyPending,
		})
		return err
	case models.OrderDeliverySceneSMSNotice:
		if intent.ReceiverUserID == nil || s.sms == nil {
			return ErrInternal("短信通知交付信息不完整")
		}
		existing, lookupErr := s.repo.SmsNotice.GetByOrderID(ctx, order.ID)
		if lookupErr != nil {
			return ErrInternal("查询短信通知记录失败")
		}
		if existing != nil {
			if existing.Status == models.SmsNoticeStatusFailed {
				_, err = s.sms.RetryByOrder(ctx, order.UserID, order.ID)
				return err
			}
			return nil
		}
		oliveBranchRecordID := 0
		if intent.OliveBranchRecordID != nil {
			oliveBranchRecordID = *intent.OliveBranchRecordID
		}
		_, err = s.sms.Send(ctx, order.UserID, SendSmsNoticeInput{
			OrderID:                 order.ID,
			ReceiverUserID:          *intent.ReceiverUserID,
			OliveBranchRecordID:     oliveBranchRecordID,
			ApplicationID:           intent.ApplicationID,
			MemberRemovalID:         intent.MemberRemovalID,
			NoticeType:              intent.NoticeType,
			ProjectID:               intent.ProjectID,
			OrderPushAlreadyPending: true,
		})
		return err
	default:
		return ErrInternal("不支持的订单交付场景")
	}
}

func (s *PaidOrderDeliveryService) retryLegacyOrder(ctx context.Context, order *models.Order) error {
	promotion, err := s.repo.EmailPromotion.GetByOrderID(ctx, order.ID)
	if err != nil {
		return ErrInternal("查询邮件推广记录失败")
	}
	if promotion != nil {
		if s.email == nil {
			return ErrInternal("邮件推广服务不可用")
		}
		_, err = s.email.TriggerPromotionWithInput(ctx, order.UserID, TriggerPromotionInput{
			OrderID: order.ID, ProjectID: promotion.ProjectID, Strategy: promotion.Strategy, OrderPushAlreadyPending: true,
		})
		return err
	}
	if s.sms == nil {
		return ErrInternal("短信通知服务不可用")
	}
	_, err = s.sms.RetryByOrder(ctx, order.UserID, order.ID)
	return err
}
