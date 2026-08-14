package wechat

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

const virtualPaymentMethod = "requestVirtualPayment"

// VirtualPayConfig contains server-only Mini Program virtual-payment credentials.
type VirtualPayConfig struct {
	OfferID string
	AppKey  string
	Env     int
}

// DefaultVirtualPayConfig loads the official Mini Program virtual-payment configuration.
func DefaultVirtualPayConfig() (*VirtualPayConfig, error) {
	offerID := strings.TrimSpace(os.Getenv("WECHAT_VIRTUAL_PAY_OFFER_ID"))
	appKey := strings.TrimSpace(os.Getenv("WECHAT_VIRTUAL_PAY_APP_KEY"))
	env := 0
	if raw := strings.TrimSpace(os.Getenv("WECHAT_VIRTUAL_PAY_ENV")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || (parsed != 0 && parsed != 1) {
			return nil, fmt.Errorf("WECHAT_VIRTUAL_PAY_ENV must be 0 or 1")
		}
		env = parsed
	}
	if offerID == "" {
		return nil, fmt.Errorf("WECHAT_VIRTUAL_PAY_OFFER_ID not configured")
	}
	if appKey == "" {
		return nil, fmt.Errorf("WECHAT_VIRTUAL_PAY_APP_KEY not configured")
	}
	return &VirtualPayConfig{OfferID: offerID, AppKey: appKey, Env: env}, nil
}

type virtualPaymentSignData struct {
	OfferID      string `json:"offerId"`
	BuyQuantity  int    `json:"buyQuantity"`
	Env          int    `json:"env"`
	CurrencyType string `json:"currencyType"`
	ProductID    string `json:"productId"`
	GoodsPrice   int    `json:"goodsPrice"`
	OutTradeNo   string `json:"outTradeNo"`
	Attach       string `json:"attach"`
}

// VirtualPaymentParams are passed directly to wx.requestVirtualPayment.
type VirtualPaymentParams struct {
	SignData  string `json:"signData"`
	PaySig    string `json:"paySig"`
	Signature string `json:"signature"`
	Mode      string `json:"mode"`
}

func hmacSHA256Hex(key, value string) string {
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

// CreateVirtualPaymentParams signs a direct-purchase request with the AppKey and current session_key.
func CreateVirtualPaymentParams(config *VirtualPayConfig, sessionKey string, orderID int, createdAt time.Time, productID int, unitPrice float64, quantity int) (*VirtualPaymentParams, error) {
	if config == nil || config.OfferID == "" || config.AppKey == "" {
		return nil, fmt.Errorf("virtual payment is not configured")
	}
	if strings.TrimSpace(sessionKey) == "" {
		return nil, fmt.Errorf("session_key is empty")
	}
	if orderID <= 0 || productID <= 0 || quantity <= 0 {
		return nil, fmt.Errorf("invalid virtual payment order")
	}
	goodsPrice := int(math.Round(unitPrice * 100))
	if goodsPrice <= 0 {
		return nil, fmt.Errorf("invalid virtual payment price")
	}
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	outTradeNo := fmt.Sprintf("KZ%d_%d", createdAt.Unix(), orderID)
	attach, err := json.Marshal(struct {
		OrderID int `json:"orderId"`
	}{OrderID: orderID})
	if err != nil {
		return nil, fmt.Errorf("marshal virtual payment attach: %w", err)
	}
	signDataBytes, err := json.Marshal(virtualPaymentSignData{
		OfferID:      config.OfferID,
		BuyQuantity:  quantity,
		Env:          config.Env,
		CurrencyType: "CNY",
		ProductID:    strconv.Itoa(productID),
		GoodsPrice:   goodsPrice,
		OutTradeNo:   outTradeNo,
		Attach:       string(attach),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal virtual payment signData: %w", err)
	}
	signData := string(signDataBytes)
	return &VirtualPaymentParams{
		SignData:  signData,
		PaySig:    hmacSHA256Hex(config.AppKey, virtualPaymentMethod+"&"+signData),
		Signature: hmacSHA256Hex(sessionKey, signData),
		Mode:      "short_series_goods",
	}, nil
}
