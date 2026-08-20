package handler

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/wechat"
	"github.com/labstack/echo/v4"
)

type virtualPaymentGoodsInfo struct {
	ProductID   string `json:"ProductId" xml:"ProductId"`
	Quantity    int    `json:"Quantity" xml:"Quantity"`
	OrigPrice   int    `json:"OrigPrice" xml:"OrigPrice"`
	ActualPrice int    `json:"ActualPrice" xml:"ActualPrice"`
	Attach      string `json:"Attach" xml:"Attach"`
}

type virtualPaymentWechatPayInfo struct {
	MchOrderNo    string `json:"MchOrderNo" xml:"MchOrderNo"`
	TransactionID string `json:"TransactionId" xml:"TransactionId"`
	PaidTime      int64  `json:"PaidTime" xml:"PaidTime"`
}

type virtualPaymentMessage struct {
	Event         string                      `json:"Event" xml:"Event"`
	OpenID        string                      `json:"OpenId" xml:"OpenId"`
	OutTradeNo    string                      `json:"OutTradeNo" xml:"OutTradeNo"`
	Env           int                         `json:"Env" xml:"Env"`
	WechatPayInfo virtualPaymentWechatPayInfo `json:"WeChatPayInfo" xml:"WeChatPayInfo"`
	GoodsInfo     virtualPaymentGoodsInfo     `json:"GoodsInfo" xml:"GoodsInfo"`
	ProvideStatus string                      `json:"provide_status" xml:"provide_status"`
	PayOrderID    string                      `json:"pay_order_id" xml:"pay_order_id"`
	IOSProductID  string                      `json:"product_id" xml:"product_id"`
	ProductCount  string                      `json:"p_count" xml:"p_count"`
}

type virtualPaymentCallbackResponse struct {
	XMLName xml.Name `json:"-" xml:"xml"`
	ErrCode int      `json:"ErrCode" xml:"ErrCode"`
	ErrMsg  string   `json:"ErrMsg" xml:"ErrMsg"`
}

// matchesVirtualPaymentPrices keeps the existing unit-price callback behavior
// compatible while also accepting a line-total callback. Both callback fields
// must use the same semantic so a mixed or partial amount is never accepted.
func matchesVirtualPaymentPrices(goods virtualPaymentGoodsInfo, unitPrice, totalPrice int) bool {
	if unitPrice <= 0 || totalPrice <= 0 {
		return false
	}
	return (goods.OrigPrice == unitPrice && goods.ActualPrice == unitPrice) ||
		(goods.OrigPrice == totalPrice && goods.ActualPrice == totalPrice)
}

type iosRefundQueryResponse struct {
	XMLName    xml.Name `json:"-" xml:"xml"`
	ResultCode int      `json:"result_code" xml:"result_code"`
	ResultInfo string   `json:"result_info" xml:"result_info"`
	Evidence   string   `json:"evidence" xml:"evidence"`
}

func verifyWechatMessageSignature(token, timestamp, nonce, signature string) bool {
	if token == "" || timestamp == "" || nonce == "" || signature == "" {
		return false
	}
	parts := []string{token, timestamp, nonce}
	sort.Strings(parts)
	sum := sha1.Sum([]byte(strings.Join(parts, "")))
	return strings.EqualFold(hex.EncodeToString(sum[:]), signature)
}

func bindVirtualPaymentMessage(body []byte, contentType string) (virtualPaymentMessage, bool, error) {
	var message virtualPaymentMessage
	isXML := strings.Contains(strings.ToLower(contentType), "xml") || strings.HasPrefix(strings.TrimSpace(string(body)), "<")
	if isXML {
		return message, true, xml.Unmarshal(body, &message)
	}
	return message, false, json.Unmarshal(body, &message)
}

func writeVirtualPaymentCallback(ctx echo.Context, isXML bool, code int, message string) error {
	response := virtualPaymentCallbackResponse{ErrCode: code, ErrMsg: message}
	if isXML {
		return ctx.XML(http.StatusOK, response)
	}
	return ctx.JSON(http.StatusOK, response)
}

func buildIOSRefundQueryResponse(message virtualPaymentMessage) iosRefundQueryResponse {
	identity := "pay_order_id=" + strings.TrimSpace(message.PayOrderID) +
		", product_id=" + strings.TrimSpace(message.IOSProductID) +
		", p_count=" + strings.TrimSpace(message.ProductCount)
	switch strings.TrimSpace(message.ProvideStatus) {
	case "0":
		return iosRefundQueryResponse{
			ResultCode: 0,
			ResultInfo: "未发货，建议退款",
			Evidence:   "微信虚拟支付发货状态为未发货；" + identity,
		}
	case "1":
		return iosRefundQueryResponse{
			ResultCode: 1,
			ResultInfo: "已发货，拒绝退款",
			Evidence:   "微信虚拟支付发货状态为已发货，虚拟权益或服务已交付；" + identity,
		}
	case "2":
		return iosRefundQueryResponse{
			ResultCode: 1,
			ResultInfo: "发货中，拒绝退款",
			Evidence:   "微信虚拟支付发货状态为发货中，虚拟权益或服务正在交付；" + identity,
		}
	default:
		return iosRefundQueryResponse{
			ResultCode: 1,
			ResultInfo: "发货状态异常，暂不建议退款",
			Evidence:   "退款问询未提供有效发货状态；provide_status=" + strings.TrimSpace(message.ProvideStatus) + "; " + identity,
		}
	}
}

func writeIOSRefundQueryResponse(ctx echo.Context, isXML bool, response iosRefundQueryResponse) error {
	if isXML {
		return ctx.XML(http.StatusOK, response)
	}
	return ctx.JSON(http.StatusOK, response)
}

// VerifyVirtualPaymentCallback handles the WeChat message-server URL verification handshake.
func (s *Server) VerifyVirtualPaymentCallback(ctx echo.Context) error {
	token := strings.TrimSpace(os.Getenv("WECHAT_VIRTUAL_PAY_MESSAGE_TOKEN"))
	if !verifyWechatMessageSignature(token, ctx.QueryParam("timestamp"), ctx.QueryParam("nonce"), ctx.QueryParam("signature")) {
		return ctx.NoContent(http.StatusForbidden)
	}
	return ctx.String(http.StatusOK, ctx.QueryParam("echostr"))
}

// VirtualPaymentCallback consumes official xpay_goods_deliver_notify events.
func (s *Server) VirtualPaymentCallback(ctx echo.Context) error {
	token := strings.TrimSpace(os.Getenv("WECHAT_VIRTUAL_PAY_MESSAGE_TOKEN"))
	if !verifyWechatMessageSignature(token, ctx.QueryParam("timestamp"), ctx.QueryParam("nonce"), ctx.QueryParam("signature")) {
		return ctx.NoContent(http.StatusForbidden)
	}
	body, err := io.ReadAll(io.LimitReader(ctx.Request().Body, 1<<20))
	if err != nil {
		return ctx.NoContent(http.StatusBadRequest)
	}
	message, isXML, err := bindVirtualPaymentMessage(body, ctx.Request().Header.Get(echo.HeaderContentType))
	if err != nil {
		log.Printf("[VirtualPaymentCallback] invalid message: %v", err)
		return writeVirtualPaymentCallback(ctx, isXML, 1, "invalid message")
	}
	if message.Event == "xpay_subscribe_ios_refund_query_notify" ||
		(message.Event == "" && message.PayOrderID != "" && message.ProvideStatus != "") {
		response := buildIOSRefundQueryResponse(message)
		log.Printf("[VirtualPaymentCallback] iOS refund query decision, pay_order_id=%s provide_status=%s result_code=%d", message.PayOrderID, message.ProvideStatus, response.ResultCode)
		return writeIOSRefundQueryResponse(ctx, isXML, response)
	}
	if message.Event != "xpay_goods_deliver_notify" {
		return writeVirtualPaymentCallback(ctx, isXML, 0, "success")
	}
	orderID, err := wechat.ParseOrderIDFromOutTradeNo(message.OutTradeNo)
	if err != nil {
		log.Printf("[VirtualPaymentCallback] invalid out_trade_no: %v", err)
		return writeVirtualPaymentCallback(ctx, isXML, 1, "invalid order")
	}
	order, err := s.svc.Payment.GetOrder(ctx.Request().Context(), orderID)
	if err != nil || order == nil {
		return writeVirtualPaymentCallback(ctx, isXML, 1, "order not found")
	}
	if order.Status == models.OrderStatusPaid {
		s.svc.Payment.EnsurePaidOrderDelivery(ctx.Request().Context(), order)
		return writeVirtualPaymentCallback(ctx, isXML, 0, "success")
	}
	productID, parseErr := strconv.Atoi(message.GoodsInfo.ProductID)
	product, productErr := s.repo.Product.GetByID(ctx.Request().Context(), order.ProductID)
	user, userErr := s.repo.User.GetByID(ctx.Request().Context(), order.UserID)
	var attach struct {
		OrderID int `json:"orderId"`
	}
	attachErr := json.Unmarshal([]byte(message.GoodsInfo.Attach), &attach)
	expectedEnv, envErr := strconv.Atoi(strings.TrimSpace(os.Getenv("WECHAT_VIRTUAL_PAY_ENV")))
	if strings.TrimSpace(os.Getenv("WECHAT_VIRTUAL_PAY_ENV")) == "" {
		expectedEnv, envErr = 0, nil
	}
	// Validate callback prices against the immutable order snapshot. Product
	// catalog prices may change while an already-created order is being paid.
	expectedUnitPrice := int(math.Round(order.Price * 100))
	expectedTotalPrice := int(math.Round(order.ActualPaid * 100))
	pricesMatch := matchesVirtualPaymentPrices(message.GoodsInfo, expectedUnitPrice, expectedTotalPrice)
	if parseErr != nil || productID != order.ProductID || message.GoodsInfo.Quantity != order.Quantity ||
		productErr != nil || product == nil || !pricesMatch ||
		attachErr != nil || attach.OrderID != order.ID || envErr != nil || message.Env != expectedEnv ||
		userErr != nil || user == nil || user.OpenID != message.OpenID {
		log.Printf("[VirtualPaymentCallback] order validation failed, order_id=%d product_id=%q expected_product_id=%d quantity=%d expected_quantity=%d orig_price=%d actual_price=%d expected_unit_price=%d expected_total_price=%d env=%d expected_env=%d",
			orderID, message.GoodsInfo.ProductID, order.ProductID, message.GoodsInfo.Quantity, order.Quantity,
			message.GoodsInfo.OrigPrice, message.GoodsInfo.ActualPrice, expectedUnitPrice, expectedTotalPrice,
			message.Env, expectedEnv)
		return writeVirtualPaymentCallback(ctx, isXML, 1, "order mismatch")
	}
	transactionID := strings.TrimSpace(message.WechatPayInfo.TransactionID)
	if transactionID == "" {
		transactionID = "XPAY:" + message.OutTradeNo
	}
	payTime := time.Now()
	if message.WechatPayInfo.PaidTime > 0 {
		payTime = time.Unix(message.WechatPayInfo.PaidTime, 0)
	}
	if err := s.svc.Payment.ProcessVirtualPayment(ctx.Request().Context(), order, transactionID, payTime); err != nil {
		log.Printf("[VirtualPaymentCallback] process order %d: %v", orderID, err)
		return writeVirtualPaymentCallback(ctx, isXML, 1, "delivery failed")
	}
	order.Status = models.OrderStatusPaid
	s.svc.Payment.EnsurePaidOrderDelivery(ctx.Request().Context(), order)
	return writeVirtualPaymentCallback(ctx, isXML, 0, "success")
}
