package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/service"
	"github.com/kuaizu-team/kuaizu-service/internal/wechat"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments"
)

type wechatPayResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func successResponse() wechatPayResponse {
	return wechatPayResponse{Code: "SUCCESS", Message: "成功"}
}

func failResponse(message string) wechatPayResponse {
	return wechatPayResponse{Code: "FAIL", Message: message}
}

// extractPaymentInfo extracts payment time and transaction ID from transaction
func extractPaymentInfo(transaction *payments.Transaction) (time.Time, string) {
	var payTime time.Time
	if transaction.SuccessTime != nil {
		payTime, _ = time.Parse(time.RFC3339, *transaction.SuccessTime)
	}
	if payTime.IsZero() {
		payTime = time.Now()
	}

	transactionID := ""
	if transaction.TransactionId != nil {
		transactionID = *transaction.TransactionId
	}

	return payTime, transactionID
}

// WechatPayCallback handles POST /payment/wechat/notify
func (s *Server) WechatPayCallback(ctx echo.Context) error {
	transaction, err := s.svc.Payment.ParseNotification(ctx.Request().Context(), ctx.Request())
	if err != nil {
		var svcErr *service.ServiceError
		if errors.As(err, &svcErr) && svcErr.Code == service.ErrCodeBadRequest {
			return ctx.JSON(http.StatusBadRequest, failResponse(svcErr.Message))
		}
		return ctx.JSON(http.StatusInternalServerError, failResponse("支付配置错误"))
	}

	// Validate transaction
	if transaction.OutTradeNo == nil {
		return ctx.JSON(http.StatusOK, successResponse())
	}

	orderID, err := wechat.ParseOrderIDFromOutTradeNo(*transaction.OutTradeNo)
	if err != nil {
		return ctx.JSON(http.StatusOK, successResponse())
	}

	if transaction.TradeState == nil || *transaction.TradeState != "SUCCESS" {
		s.svc.Payment.MarkPaymentFailed(ctx.Request().Context(), orderID)
		return ctx.JSON(http.StatusOK, successResponse())
	}

	// Delegate business logic to service
	order, err := s.svc.Payment.GetOrder(ctx.Request().Context(), orderID)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, failResponse("查询订单失败"))
	}
	if order == nil {
		return ctx.JSON(http.StatusOK, successResponse())
	}

	if order.Status == models.OrderStatusPaid {
		return ctx.JSON(http.StatusOK, successResponse())
	}

	payTime, transactionID := extractPaymentInfo(transaction)

	if err := s.svc.Payment.ProcessPayment(ctx.Request().Context(), order, transactionID, payTime); err != nil {
		return ctx.JSON(http.StatusInternalServerError, failResponse("处理支付失败"))
	}

	return ctx.JSON(http.StatusOK, successResponse())
}
