package wechat

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDefaultVirtualPayConfigRequiresMessageToken(t *testing.T) {
	t.Setenv("WECHAT_VIRTUAL_PAY_OFFER_ID", "offer")
	t.Setenv("WECHAT_VIRTUAL_PAY_APP_KEY", "app-key")
	t.Setenv("WECHAT_VIRTUAL_PAY_ENV", "0")
	t.Setenv("WECHAT_VIRTUAL_PAY_MESSAGE_TOKEN", "")

	config, err := DefaultVirtualPayConfig()
	require.Nil(t, config)
	require.EqualError(t, err, "WECHAT_VIRTUAL_PAY_MESSAGE_TOKEN not configured")
}

func TestDefaultVirtualPayConfigAcceptsCompleteConfig(t *testing.T) {
	t.Setenv("WECHAT_VIRTUAL_PAY_OFFER_ID", "offer")
	t.Setenv("WECHAT_VIRTUAL_PAY_APP_KEY", "app-key")
	t.Setenv("WECHAT_VIRTUAL_PAY_ENV", "1")
	t.Setenv("WECHAT_VIRTUAL_PAY_MESSAGE_TOKEN", "message-token")

	config, err := DefaultVirtualPayConfig()
	require.NoError(t, err)
	require.Equal(t, &VirtualPayConfig{OfferID: "offer", AppKey: "app-key", Env: 1}, config)
}

func TestCreateVirtualPaymentParams(t *testing.T) {
	params, err := CreateVirtualPaymentParams(
		&VirtualPayConfig{OfferID: "offer", AppKey: "app-key", Env: 0},
		"session-key",
		42,
		time.Unix(1700000000, 0),
		1,
		0.5,
		2,
	)
	require.NoError(t, err)
	require.Equal(t, "short_series_goods", params.Mode)
	require.Equal(t, `{"offerId":"offer","buyQuantity":2,"env":0,"currencyType":"CNY","productId":"1","goodsPrice":50,"outTradeNo":"KZ1700000000_42","attach":"{\"orderId\":42}"}`, params.SignData)
	require.Equal(t, "148f261403b7312caf647845fef1ae179c989ea3b661f202fe50351ac1f719db", params.PaySig)
	require.Equal(t, "d2c4eeef0aca13ea8edd17f5452e21cc4ce873c9ffc213494906982e3b08e8c3", params.Signature)
}

func TestCreateVirtualPaymentParamsRejectsInvalidInput(t *testing.T) {
	_, err := CreateVirtualPaymentParams(&VirtualPayConfig{OfferID: "offer", AppKey: "key"}, "session", 1, time.Now(), 2, 0, 1)
	require.Error(t, err)
}
