package handler

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVerifyWechatMessageSignature(t *testing.T) {
	require.True(t, verifyWechatMessageSignature("token", "1700000000", "nonce", "bf37e74fc61ce5974ce58c68e55130b79b2578b9"))
	require.False(t, verifyWechatMessageSignature("token", "1700000000", "nonce", "invalid"))
}

func TestBindVirtualPaymentMessageJSON(t *testing.T) {
	body := []byte(`{"Event":"xpay_goods_deliver_notify","OpenId":"openid","OutTradeNo":"KZ1700000000_42","Env":0,"GoodsInfo":{"ProductId":"1","Quantity":2,"Attach":"{\"orderId\":42}"}}`)
	message, isXML, err := bindVirtualPaymentMessage(body, "application/json")
	require.NoError(t, err)
	require.False(t, isXML)
	require.Equal(t, "xpay_goods_deliver_notify", message.Event)
	require.Equal(t, "openid", message.OpenID)
	require.Equal(t, "1", message.GoodsInfo.ProductID)
	require.Equal(t, 2, message.GoodsInfo.Quantity)
}

func TestBindVirtualPaymentMessageXML(t *testing.T) {
	body := []byte(`<xml><Event>xpay_goods_deliver_notify</Event><OpenId>openid</OpenId><OutTradeNo>KZ1700000000_42</OutTradeNo><Env>0</Env><GoodsInfo><ProductId>12</ProductId><Quantity>1</Quantity><Attach>{"orderId":42}</Attach></GoodsInfo></xml>`)
	message, isXML, err := bindVirtualPaymentMessage(body, "text/xml")
	require.NoError(t, err)
	require.True(t, isXML)
	require.Equal(t, "12", message.GoodsInfo.ProductID)
	require.Equal(t, `{"orderId":42}`, message.GoodsInfo.Attach)
}

func TestMatchesVirtualPaymentPrices(t *testing.T) {
	tests := []struct {
		name       string
		goods      virtualPaymentGoodsInfo
		unitPrice  int
		totalPrice int
		wantMatch  bool
	}{
		{
			name:      "unit price callback remains compatible",
			goods:     virtualPaymentGoodsInfo{OrigPrice: 10, ActualPrice: 10},
			unitPrice: 10,
			totalPrice: 100,
			wantMatch: true,
		},
		{
			name:      "line total callback",
			goods:     virtualPaymentGoodsInfo{OrigPrice: 100, ActualPrice: 100},
			unitPrice: 10,
			totalPrice: 100,
			wantMatch: true,
		},
		{
			name:      "mixed price semantics",
			goods:     virtualPaymentGoodsInfo{OrigPrice: 10, ActualPrice: 100},
			unitPrice: 10,
			totalPrice: 100,
			wantMatch: false,
		},
		{
			name:      "unexpected amount",
			goods:     virtualPaymentGoodsInfo{OrigPrice: 90, ActualPrice: 90},
			unitPrice: 10,
			totalPrice: 100,
			wantMatch: false,
		},
		{
			name:      "invalid total",
			goods:     virtualPaymentGoodsInfo{OrigPrice: 10, ActualPrice: 10},
			unitPrice: 10,
			totalPrice: 0,
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.wantMatch, matchesVirtualPaymentPrices(tt.goods, tt.unitPrice, tt.totalPrice))
		})
	}
}

func TestBuildIOSRefundQueryResponse(t *testing.T) {
	tests := []struct {
		name          string
		provideStatus string
		wantCode      int
		wantInfo      string
	}{
		{name: "not delivered", provideStatus: "0", wantCode: 0, wantInfo: "未发货，建议退款"},
		{name: "delivered", provideStatus: "1", wantCode: 1, wantInfo: "已发货，拒绝退款"},
		{name: "delivering", provideStatus: "2", wantCode: 1, wantInfo: "发货中，拒绝退款"},
		{name: "invalid", provideStatus: "", wantCode: 1, wantInfo: "发货状态异常，暂不建议退款"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := buildIOSRefundQueryResponse(virtualPaymentMessage{
				ProvideStatus: tt.provideStatus,
				PayOrderID:    "pay-42",
				IOSProductID:  "12",
				ProductCount:  "1",
			})
			require.Equal(t, tt.wantCode, response.ResultCode)
			require.Equal(t, tt.wantInfo, response.ResultInfo)
			require.Contains(t, response.Evidence, "pay_order_id=pay-42")
		})
	}
}

func TestBindIOSRefundQueryMessage(t *testing.T) {
	body := []byte(`{"Event":"xpay_subscribe_ios_refund_query_notify","provide_status":"1","pay_order_id":"pay-42","product_id":"12","p_count":"1"}`)
	message, isXML, err := bindVirtualPaymentMessage(body, "application/json")
	require.NoError(t, err)
	require.False(t, isXML)
	require.Equal(t, "xpay_subscribe_ios_refund_query_notify", message.Event)
	require.Equal(t, "1", message.ProvideStatus)
	require.Equal(t, "pay-42", message.PayOrderID)
}
