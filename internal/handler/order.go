package handler

import (
	"strings"

	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/service"
	"github.com/labstack/echo/v4"
)

type applyRefundRequest struct {
	Reason string `json:"reason"`
}

// ListMyOrders handles GET /orders/me
func (s *Server) ListMyOrders(ctx echo.Context, params api.ListMyOrdersParams) error {
	userID := GetUserID(ctx)

	// Build list params
	listParams := repository.OrderListParams{
		UserID: userID,
		Page:   1,
		Size:   10,
	}

	if params.Page != nil {
		listParams.Page = int(*params.Page)
	}
	if params.Size != nil {
		listParams.Size = int(*params.Size)
	}
	if listParams.Page < 1 {
		listParams.Page = 1
	}
	if listParams.Size < 1 || listParams.Size > 100 {
		listParams.Size = 10
	}

	if params.Status != nil {
		status := int(*params.Status)
		listParams.Status = &status
	}
	if params.RefundStatus != nil {
		refundStatus := int(*params.RefundStatus)
		listParams.RefundStatus = &refundStatus
	}
	if params.AfterSale != nil {
		listParams.AfterSale = *params.AfterSale
	}

	// Query (list stays as simple passthrough)
	orders, total, err := s.repo.Order.ListByUserID(ctx.Request().Context(), listParams)
	if err != nil {
		return InternalError(ctx, "获取订单列表失败")
	}

	// Convert to VOs
	list := make([]api.OrderVO, len(orders))
	for i, o := range orders {
		list[i] = *o.ToVO()
	}

	// Build pagination info
	totalPages := int((total + int64(listParams.Size) - 1) / int64(listParams.Size))
	pageInfo := api.PageInfo{
		Page:       &listParams.Page,
		Size:       &listParams.Size,
		Total:      &total,
		TotalPages: &totalPages,
	}

	return Success(ctx, struct {
		List     *[]api.OrderVO `json:"list"`
		PageInfo *api.PageInfo  `json:"pageInfo"`
	}{
		List:     &list,
		PageInfo: &pageInfo,
	})
}

// CreateOrder handles POST /orders
func (s *Server) CreateOrder(ctx echo.Context) error {
	userID := GetUserID(ctx)

	var reqItem api.CreateOrderDTO
	if err := ctx.Bind(&reqItem); err != nil {
		return BadRequest(ctx, "请求参数错误")
	}

	// Convert API DTO to service DTO
	item := service.CreateOrderItem{
		ProductID: reqItem.ProductId,
		Quantity:  reqItem.Quantity,
	}

	createdOrder, err := s.svc.Order.CreateOrder(ctx.Request().Context(), userID, item)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	return Success(ctx, createdOrder.ToVO())
}

// GetOrder handles GET /orders/{id}
func (s *Server) GetOrder(ctx echo.Context, id int) error {
	userID := GetUserID(ctx)

	order, err := s.svc.Order.GetOrder(ctx.Request().Context(), userID, id)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	return Success(ctx, order.ToVO())
}

// InitiatePayment handles POST /orders/{id}/pay
func (s *Server) InitiatePayment(ctx echo.Context, id int) error {
	userID := GetUserID(ctx)
	openID := GetOpenID(ctx)

	paymentParams, err := s.svc.Order.InitiatePayment(ctx.Request().Context(), userID, openID, id)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	return Success(ctx, api.WechatPaymentParams{
		TimeStamp: &paymentParams.TimeStamp,
		NonceStr:  &paymentParams.NonceStr,
		Package:   &paymentParams.Package,
		SignType:  &paymentParams.SignType,
		PaySign:   &paymentParams.PaySign,
	})
}

// CancelOrder handles POST /orders/{id}/cancel
func (s *Server) CancelOrder(ctx echo.Context, id int) error {
	userID := GetUserID(ctx)

	order, err := s.svc.Order.CancelOrder(ctx.Request().Context(), userID, id)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	return Success(ctx, order.ToVO())
}

// ApplyOrderRefund handles generated POST /orders/{id}/refund/apply routes.
func (s *Server) ApplyOrderRefund(ctx echo.Context, id int) error {
	return s.applyOrderRefund(ctx, id)
}

func (s *Server) applyOrderRefund(ctx echo.Context, id int) error {
	userID := GetUserID(ctx)
	var req applyRefundRequest
	if err := ctx.Bind(&req); err != nil {
		return BadRequest(ctx, "请求参数错误")
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Reason == "" {
		return BadRequest(ctx, "退款原因不能为空")
	}

	order, err := s.svc.Order.ApplyRefund(ctx.Request().Context(), userID, id, req.Reason)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	return Success(ctx, order.ToVO())
}

// WithdrawOrderRefund handles PATCH /orders/{id}/refund/withdraw.
func (s *Server) WithdrawOrderRefund(ctx echo.Context, id int) error {
	userID := GetUserID(ctx)
	order, err := s.svc.Order.WithdrawRefund(ctx.Request().Context(), userID, id)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	return Success(ctx, order.ToVO())
}
