package service

import (
	"context"
	"fmt"

	"github.com/trv3wood/kuaizu-server/internal/models"
	"github.com/trv3wood/kuaizu-server/internal/repository"
	"github.com/trv3wood/kuaizu-server/internal/wechat"
)

// OrderService handles order-related business logic.
type OrderService struct {
	repo *repository.Repository
}

// NewOrderService creates a new OrderService.
func NewOrderService(repo *repository.Repository) *OrderService {
	return &OrderService{repo: repo}
}

// CreateOrderItem is the input DTO for creating an order.
type CreateOrderItem struct {
	ProductID int
	Quantity  int
}

// CreateOrder validates products, calculates price, and creates an order.
func (s *OrderService) CreateOrder(ctx context.Context, userID int, items []CreateOrderItem) (*models.Order, error) {
	if len(items) == 0 {
		return nil, ErrBadRequest("订单商品不能为空")
	}

	var actualPaid float64
	var orderItems []*models.OrderItem

	for _, item := range items {
		if item.ProductID <= 0 {
			return nil, ErrBadRequest("商品ID无效")
		}
		if item.Quantity <= 0 {
			return nil, ErrBadRequest("购买数量必须大于0")
		}

		product, err := s.repo.Product.GetByID(ctx, item.ProductID)
		if err != nil {
			return nil, ErrInternal("获取商品信息失败")
		}
		if product == nil {
			return nil, ErrNotFound(fmt.Sprintf("商品ID %d 不存在", item.ProductID))
		}

		actualPaid += product.Price * float64(item.Quantity)

		orderItems = append(orderItems, &models.OrderItem{
			ProductID:   item.ProductID,
			Price:       product.Price,
			Quantity:    item.Quantity,
			ProductName: &product.Name,
		})
	}

	order := &models.Order{
		UserID:     userID,
		ActualPaid: actualPaid,
		Status:     0, // 待支付
		Items:      orderItems,
	}

	createdOrder, err := s.repo.Order.Create(ctx, order)
	if err != nil {
		return nil, ErrInternal("创建订单失败")
	}

	return createdOrder, nil
}

// GetOrder retrieves an order with ownership check.
func (s *OrderService) GetOrder(ctx context.Context, userID, orderID int) (*models.Order, error) {
	order, err := s.repo.Order.GetByID(ctx, orderID)
	if err != nil {
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
		return nil, ErrInternal("获取订单详情失败")
	}
	if order == nil {
		return nil, ErrNotFound("订单不存在")
	}
	if order.UserID != userID {
		return nil, ErrForbidden("无权操作此订单")
	}
	if order.Status != 0 {
		return nil, ErrBadRequest("订单状态不允许支付")
	}

	payConfig, err := wechat.DefaultPayConfig()
	if err != nil {
		return nil, ErrInternal("支付配置错误: " + err.Error())
	}

	payClient, err := wechat.NewPayClient(payConfig)
	if err != nil {
		return nil, ErrInternal("初始化支付客户端失败: " + err.Error())
	}

	description := "快组校园商品购买"
	if len(order.Items) > 0 && order.Items[0].ProductName != nil {
		description = *order.Items[0].ProductName
	}

	outTradeNo := wechat.GenerateOutTradeNo(order.ID)
	amountCents := int(order.ActualPaid * 100)

	paymentParams, err := payClient.CreatePrepayOrderWithPayment(
		ctx,
		outTradeNo,
		description,
		openID,
		amountCents,
	)
	if err != nil {
		return nil, ErrInternal("创建支付订单失败: " + err.Error())
	}

	return paymentParams, nil
}

// CancelOrder cancels an unpaid order (status must be 0).
func (s *OrderService) CancelOrder(ctx context.Context, userID, orderID int) (*models.Order, error) {
	order, err := s.repo.Order.GetByID(ctx, orderID)
	if err != nil {
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

	if err := s.repo.Order.UpdateStatus(ctx, orderID, 2); err != nil {
		return nil, ErrInternal("取消订单失败")
	}

	// Re-fetch to return updated order
	updated, err := s.repo.Order.GetByID(ctx, orderID)
	if err != nil {
		return nil, ErrInternal("获取更新后的订单失败")
	}

	return updated, nil
}
