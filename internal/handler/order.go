package handler

import (
	"github.com/labstack/echo/v4"
	"github.com/trv3wood/kuaizu-server/api"
	"github.com/trv3wood/kuaizu-server/internal/repository"
	"github.com/trv3wood/kuaizu-server/internal/service"
)

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

	var reqItems []api.CreateOrderDTO
	if err := ctx.Bind(&reqItems); err != nil {
		return BadRequest(ctx, "请求参数错误")
	}

	// Convert API DTO to service DTO
	items := make([]service.CreateOrderItem, len(reqItems))
	for i, item := range reqItems {
		items[i] = service.CreateOrderItem{
			ProductID: item.ProductId,
			Quantity:  item.Quantity,
		}
	}

	createdOrder, err := s.svc.Order.CreateOrder(ctx.Request().Context(), userID, items)
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
