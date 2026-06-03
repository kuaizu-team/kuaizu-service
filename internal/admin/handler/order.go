package handler

import (
	"strconv"
	"strings"

	adminvo "github.com/kuaizu-team/kuaizu-service/internal/admin/vo"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
)

type adminRefundApplyRequest struct {
	Reason              string `json:"reason"`
	RefundApplicantType int    `json:"refundApplicantType"`
}

type adminRefundReviewRequest struct {
	RefundStatus int     `json:"refundStatus"`
	Reason       *string `json:"reason"`
	RejectReason *string `json:"rejectReason"`
}

type adminRefundRejectRequest struct {
	Reason string `json:"reason"`
}

// ListOrders handles GET /admin/orders.
func (s *AdminServer) ListOrders(ctx echo.Context) error {
	if adminRole(ctx) == models.AdminRoleSchoolAdmin {
		return response.Forbidden(ctx, "permission denied")
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
	if v := ctx.QueryParam("settlementStatus"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return response.BadRequest(ctx, "invalid settlementStatus")
		}
		params.SettlementStatus = &n
	}
	if v := ctx.QueryParam("refundStatus"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return response.BadRequest(ctx, "invalid refundStatus")
		}
		params.RefundStatus = &n
	}
	if v := ctx.QueryParam("refundApplicantType"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return response.BadRequest(ctx, "invalid refundApplicantType")
		}
		params.RefundApplicantType = &n
	}
	if v := ctx.QueryParam("userId"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return response.BadRequest(ctx, "invalid userId")
		}
		params.UserID = &n
	}

	if sid := adminSchoolID(ctx); sid != nil {
		params.SchoolID = sid
	}

	orders, total, err := s.repo.Order.AdminList(ctx.Request().Context(), params)
	if err != nil {
		return response.InternalError(ctx, "get order list failed")
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

// GetOrder handles GET /admin/orders/:id.
func (s *AdminServer) GetOrder(ctx echo.Context) error {
	if adminRole(ctx) == models.AdminRoleSchoolAdmin {
		return response.Forbidden(ctx, "permission denied")
	}

	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid order id")
	}

	order, err := s.repo.Order.AdminGetByID(ctx.Request().Context(), id)
	if err != nil {
		return response.InternalError(ctx, "get order detail failed")
	}
	if order == nil {
		return response.NotFound(ctx, "order not found")
	}

	if sid := adminSchoolID(ctx); sid != nil {
		if order.UserSchoolID == nil || *order.UserSchoolID != *sid {
			return response.Forbidden(ctx, "permission denied")
		}
	}

	return response.Success(ctx, adminvo.NewAdminOrderDetailVO(order))
}

// ApplyOrderRefund handles POST /admin/orders/:id/refund/apply.
func (s *AdminServer) ApplyOrderRefund(ctx echo.Context) error {
	if adminRole(ctx) != models.AdminRoleSchoolSuperAdmin {
		return response.Forbidden(ctx, "permission denied")
	}
	sid := adminSchoolID(ctx)
	if sid == nil {
		return response.Forbidden(ctx, "current admin has no school")
	}

	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid order id")
	}

	var req adminRefundApplyRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	reason := strings.TrimSpace(req.Reason)
	if len([]rune(reason)) < 5 || len([]rune(reason)) > 200 {
		return response.BadRequest(ctx, "refund reason must be 5-200 chars")
	}
	if req.RefundApplicantType != 1 {
		return response.BadRequest(ctx, "refundApplicantType must be 1")
	}

	order, err := s.repo.Order.AdminGetByID(ctx.Request().Context(), id)
	if err != nil {
		return response.InternalError(ctx, "get order detail failed")
	}
	if order == nil {
		return response.NotFound(ctx, "order not found")
	}
	if order.UserSchoolID == nil || *order.UserSchoolID != *sid {
		return response.Forbidden(ctx, "permission denied")
	}
	if order.Status != models.OrderStatusPaid {
		return response.BadRequest(ctx, "unpaid order cannot apply refund")
	}
	if order.RefundStatus != 0 {
		return response.BadRequest(ctx, "order already has refund status")
	}

	adminID := currentAdminID(ctx)
	ok, err := s.repo.Order.UpdateRefundApply(ctx.Request().Context(), id, reason, 1, &adminID)
	if err != nil {
		return response.InternalError(ctx, "apply refund failed")
	}
	if !ok {
		return response.BadRequest(ctx, "order status cannot apply refund")
	}

	updated, err := s.repo.Order.AdminGetByID(ctx.Request().Context(), id)
	if err != nil {
		return response.InternalError(ctx, "get updated order failed")
	}
	return response.Success(ctx, adminvo.NewAdminOrderDetailVO(updated))
}

// ReviewOrderRefund handles PATCH /admin/orders/:id/refund.
func (s *AdminServer) ReviewOrderRefund(ctx echo.Context) error {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid order id")
	}
	var req adminRefundReviewRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}

	if req.RefundStatus == 3 {
		reason := ""
		if req.Reason != nil {
			reason = *req.Reason
		}
		if strings.TrimSpace(reason) == "" && req.RejectReason != nil {
			reason = *req.RejectReason
		}
		return s.rejectOrderRefund(ctx, id, reason)
	}
	if req.RefundStatus != 2 {
		return response.BadRequest(ctx, "refundStatus must be 2 or 3")
	}
	if adminRole(ctx) != models.AdminRoleSuperAdmin {
		return response.Forbidden(ctx, "permission denied")
	}

	ok, err := s.repo.Order.AdminReviewRefund(ctx.Request().Context(), id, currentAdminID(ctx))
	if err != nil {
		return response.InternalError(ctx, "review refund failed")
	}
	if !ok {
		return response.BadRequest(ctx, "only pending refund can be reviewed")
	}

	order, err := s.repo.Order.AdminGetByID(ctx.Request().Context(), id)
	if err != nil {
		return response.InternalError(ctx, "get updated order failed")
	}
	return response.Success(ctx, adminvo.NewAdminOrderDetailVO(order))
}

// RejectOrderRefund handles PATCH/POST /admin/orders/:id/refund/reject.
func (s *AdminServer) RejectOrderRefund(ctx echo.Context) error {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid order id")
	}
	var req adminRefundRejectRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	return s.rejectOrderRefund(ctx, id, req.Reason)
}

func (s *AdminServer) rejectOrderRefund(ctx echo.Context, id int, reason string) error {
	reason = strings.TrimSpace(reason)
	if len([]rune(reason)) < 5 || len([]rune(reason)) > 200 {
		return response.BadRequest(ctx, "reject reason must be 5-200 chars")
	}

	role := adminRole(ctx)
	if role != models.AdminRoleSuperAdmin && role != models.AdminRoleSchoolSuperAdmin {
		return response.Forbidden(ctx, "permission denied")
	}

	order, err := s.repo.Order.AdminGetByID(ctx.Request().Context(), id)
	if err != nil {
		return response.InternalError(ctx, "get order detail failed")
	}
	if order == nil {
		return response.NotFound(ctx, "order not found")
	}
	if order.RefundStatus != 1 {
		return response.BadRequest(ctx, "only pending refund can be rejected")
	}
	if role == models.AdminRoleSchoolSuperAdmin {
		sid := adminSchoolID(ctx)
		if sid == nil || order.UserSchoolID == nil || *order.UserSchoolID != *sid {
			return response.Forbidden(ctx, "permission denied")
		}
		if order.RefundApplicantType == nil || *order.RefundApplicantType != 0 {
			return response.Forbidden(ctx, "school super admin can only reject consumer refund")
		}
	}

	ok, err := s.repo.Order.AdminRejectRefund(ctx.Request().Context(), id, reason, currentAdminID(ctx))
	if err != nil {
		return response.InternalError(ctx, "reject refund failed")
	}
	if !ok {
		return response.BadRequest(ctx, "only pending refund can be rejected")
	}

	updated, err := s.repo.Order.AdminGetByID(ctx.Request().Context(), id)
	if err != nil {
		return response.InternalError(ctx, "get updated order failed")
	}
	return response.Success(ctx, adminvo.NewAdminOrderDetailVO(updated))
}

// WithdrawOrderRefund handles PATCH/POST /admin/orders/:id/refund/withdraw.
func (s *AdminServer) WithdrawOrderRefund(ctx echo.Context) error {
	if adminRole(ctx) != models.AdminRoleSchoolSuperAdmin {
		return response.Forbidden(ctx, "permission denied")
	}
	sid := adminSchoolID(ctx)
	if sid == nil {
		return response.Forbidden(ctx, "current admin has no school")
	}

	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid order id")
	}

	order, err := s.repo.Order.AdminGetByID(ctx.Request().Context(), id)
	if err != nil {
		return response.InternalError(ctx, "get order detail failed")
	}
	if order == nil {
		return response.NotFound(ctx, "order not found")
	}
	if order.UserSchoolID == nil || *order.UserSchoolID != *sid {
		return response.Forbidden(ctx, "permission denied")
	}
	if order.RefundStatus != 1 {
		return response.BadRequest(ctx, "only pending refund can be withdrawn")
	}
	if order.RefundApplicantType == nil || *order.RefundApplicantType != 1 {
		return response.Forbidden(ctx, "only school admin refund can be withdrawn")
	}
	adminID := currentAdminID(ctx)
	if order.RefundApplicantAdminID == nil || *order.RefundApplicantAdminID != adminID {
		return response.Forbidden(ctx, "only applicant admin can withdraw refund")
	}

	ok, err := s.repo.Order.WithdrawRefund(ctx.Request().Context(), id)
	if err != nil {
		return response.InternalError(ctx, "withdraw refund failed")
	}
	if !ok {
		return response.BadRequest(ctx, "only pending refund can be withdrawn")
	}

	updated, err := s.repo.Order.AdminGetByID(ctx.Request().Context(), id)
	if err != nil {
		return response.InternalError(ctx, "get updated order failed")
	}
	return response.Success(ctx, adminvo.NewAdminOrderDetailVO(updated))
}
