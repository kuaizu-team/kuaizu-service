package wechat

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

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
