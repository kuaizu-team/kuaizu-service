package service

import (
	"context"
	"fmt"
	"log"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/wechat"
)

// OrderService handles order-related business logic.
type OrderService struct {
	repo       *repository.Repository
	payClient  *wechat.PayClient
	payInitErr error
}

// NewOrderService creates a new OrderService.
func NewOrderService(repo *repository.Repository, payClient *wechat.PayClient, payInitErr error) *OrderService {
	return &OrderService{
		repo:       repo,
		payClient:  payClient,
		payInitErr: payInitErr,
	}
}

// CreateOrderItem is the input DTO for creating an order.
type CreateOrderItem struct {
	ProductID int
	Quantity  int
}

// CreateOrder validates product, calculates price, and creates an order.
func (s *OrderService) CreateOrder(ctx context.Context, userID int, item CreateOrderItem) (*models.Order, error) {
	if item.ProductID <= 0 {
		return nil, ErrBadRequest("商品ID无效")
	}
	if item.Quantity <= 0 {
		return nil, ErrBadRequest("购买数量必须大于0")
	}

	product, err := s.repo.Product.GetByID(ctx, item.ProductID)
	if err != nil {
		log.Printf("[OrderService.CreateOrder] repository error getting product: %v", err)
		return nil, ErrInternal("获取商品信息失败")
	}
	if product == nil {
		return nil, ErrNotFound(fmt.Sprintf("商品ID %d 不存在", item.ProductID))
	}

	actualPaid := product.Price * float64(item.Quantity)

	order := &models.Order{
		UserID:     userID,
		ProductID:  item.ProductID,
		Price:      product.Price,
		Quantity:   item.Quantity,
		ActualPaid: actualPaid,
		Status:     models.OrderStatusPending,
	}

	createdOrder, err := s.repo.Order.Create(ctx, order)
	if err != nil {
		log.Printf("[OrderService.CreateOrder] repository error creating order: %v", err)
		return nil, ErrInternal("创建订单失败")
	}

	return createdOrder, nil
}

// GetOrder retrieves an order with ownership check.
func (s *OrderService) GetOrder(ctx context.Context, userID, orderID int) (*models.Order, error) {
	order, err := s.repo.Order.GetByID(ctx, orderID)
	if err != nil {
		log.Printf("[OrderService.GetOrder] repository error: %v", err)
		return nil, ErrInternal("获取订单详情失败")
	}
	if order == nil {
		return nil, ErrNotFound("订单不存在")
	}
	if order.UserID != userID {
		return nil, ErrForbidden("无权查看此订单")
	}
	return order, nil
}

// PaymentParams holds WeChat JSAPI payment parameters.
type PaymentParams = wechat.PaymentParams

// InitiatePayment validates the order and creates a WeChat prepay order.
func (s *OrderService) InitiatePayment(ctx context.Context, userID int, openID string, orderID int) (*PaymentParams, error) {
	if openID == "" {
		return nil, ErrBadRequest("无法获取用户OpenID")
	}

	order, err := s.repo.Order.GetByID(ctx, orderID)
	if err != nil {
		log.Printf("[OrderService.InitiatePayment] repository error getting order: %v", err)
		return nil, ErrInternal("获取订单详情失败")
	}
	if order == nil {
		return nil, ErrNotFound("订单不存在")
	}
	if order.UserID != userID {
		return nil, ErrForbidden("无权操作此订单")
	}
	if order.Status != models.OrderStatusPending {
		return nil, ErrBadRequest("订单状态不允许支付")
	}

	if s.payInitErr != nil {
		log.Printf("[OrderService.InitiatePayment] wechat pay init error: %v", s.payInitErr)
		return nil, ErrInternal("支付配置错误: " + s.payInitErr.Error())
	}
	if s.payClient == nil {
		log.Printf("[OrderService.InitiatePayment] pay client is nil")
		return nil, ErrInternal("初始化支付客户端失败")
	}

	description := "快组校园商品购买"
	if order.ProductName != nil {
		description = *order.ProductName
	}

	outTradeNo := wechat.GenerateOutTradeNo(order.ID)
	amountCents := int(order.ActualPaid * 100)

	paymentParams, err := s.payClient.CreatePrepayOrderWithPayment(
		ctx,
		outTradeNo,
		description,
		openID,
		amountCents,
	)
	if err != nil {
		log.Printf("[OrderService.InitiatePayment] wechat API error: %v", err)
		return nil, ErrInternal("创建支付订单失败: " + err.Error())
	}

	return paymentParams, nil
}

// CancelOrder cancels an unpaid order (status must be 0).
func (s *OrderService) CancelOrder(ctx context.Context, userID, orderID int) (*models.Order, error) {
	order, err := s.repo.Order.GetByID(ctx, orderID)
	if err != nil {
		log.Printf("[OrderService.CancelOrder] repository error getting order: %v", err)
		return nil, ErrInternal("获取订单详情失败")
	}
	if order == nil {
		return nil, ErrNotFound("订单不存在")
	}
	if order.UserID != userID {
		return nil, ErrForbidden("无权操作此订单")
	}
	if order.Status != 0 {
		return nil, ErrBadRequest("订单状态不允许取消")
	}

	if err := IsValidStatus("order.status", models.OrderStatusCancelled); err != nil {
		return nil, err
	}

	if err := s.repo.Order.UpdateStatus(ctx, orderID, models.OrderStatusCancelled); err != nil {
		log.Printf("[OrderService.CancelOrder] repository error updating status: %v", err)
		return nil, ErrInternal("取消订单失败")
	}

	// Re-fetch to return updated order
	updated, err := s.repo.Order.GetByID(ctx, orderID)
	if err != nil {
		log.Printf("[OrderService.CancelOrder] repository error getting updated order: %v", err)
		return nil, ErrInternal("获取更新后的订单失败")
	}

	return updated, nil
}
