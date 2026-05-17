package handler

import (
	"strconv"

	adminvo "github.com/kuaizu-team/kuaizu-service/internal/admin/vo"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
)

// ListOrders handles GET /admin/orders
func (s *AdminServer) ListOrders(ctx echo.Context) error {
	// 校区管理员（role=3）无权访问订单
	if adminRole(ctx) == models.AdminRoleSchoolAdmin {
		return response.Forbidden(ctx, "权限不足")
	}

	page, _ := strconv.Atoi(ctx.QueryParam("page"))
	size, _ := strconv.Atoi(ctx.QueryParam("size"))
	if size == 0 {
		size, _ = strconv.Atoi(ctx.QueryParam("pageSize"))
	}
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 10
	}

	params := repository.AdminOrderListParams{
		Page: page,
		Size: size,
	}

	if v := ctx.QueryParam("orderNo"); v != "" {
		params.OrderNo = &v
	}
	if v := ctx.QueryParam("wxPayNo"); v != "" {
		params.WxPayNo = &v
	}
	if v := ctx.QueryParam("nickname"); v != "" {
		params.Nickname = &v
	}
	if v := ctx.QueryParam("schoolName"); v != "" {
		params.SchoolName = &v
	}

	// 校区超级管理员（role=2）自动按学校过滤
	if sid := adminSchoolID(ctx); sid != nil {
		params.SchoolID = sid
	}

	orders, total, err := s.repo.Order.AdminList(ctx.Request().Context(), params)
	if err != nil {
		return response.InternalError(ctx, "获取订单列表失败")
	}

	list := make([]adminvo.AdminOrderVO, len(orders))
	for i, o := range orders {
		list[i] = *adminvo.NewAdminOrderVO(o)
	}

	return response.Success(ctx, map[string]interface{}{
		"list":  list,
		"total": total,
	})
}

// GetOrder handles GET /admin/orders/:id
func (s *AdminServer) GetOrder(ctx echo.Context) error {
	// 校区管理员（role=3）无权访问订单
	if adminRole(ctx) == models.AdminRoleSchoolAdmin {
		return response.Forbidden(ctx, "权限不足")
	}

	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid order id")
	}

	order, err := s.repo.Order.AdminGetByID(ctx.Request().Context(), id)
	if err != nil {
		return response.InternalError(ctx, "获取订单详情失败")
	}
	if order == nil {
		return response.NotFound(ctx, "订单不存在")
	}

	// 校区超级管理员（role=2）只能查看本校用户的订单
	if sid := adminSchoolID(ctx); sid != nil {
		if order.UserSchoolID == nil || *order.UserSchoolID != *sid {
			return response.Forbidden(ctx, "权限不足")
		}
	}

	return response.Success(ctx, adminvo.NewAdminOrderDetailVO(order))
}
