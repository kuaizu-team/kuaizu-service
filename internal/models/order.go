package models

import (
	"time"

	"github.com/kuaizu-team/kuaizu-service/api"
)

// Order represents an order in the database (wide table design)
type Order struct {
	ID         int        `db:"id"`
	UserID     int        `db:"user_id"`
	ProductID  int        `db:"product_id"`   // 商品ID
	Price      float64    `db:"price"`        // 下单时的单价快照
	Quantity   int        `db:"quantity"`     // 购买数量
	ActualPaid float64    `db:"actual_paid"`  // 实付金额
	Status     int        `db:"status"`       // 0-待支付, 1-已支付, 2-已取消, 3-已退款
	WxPayNo    *string    `db:"wx_pay_no"`    // 微信支付订单号
	OutTradeNo *string    `db:"out_trade_no"` // 商户单号
	PayTime    *time.Time `db:"pay_time"`     // 支付时间
	CreatedAt  time.Time  `db:"created_at"`   // 创建时间
	UpdatedAt  time.Time  `db:"updated_at"`   // 更新时间

	// Joined fields from product table
	ProductName *string `db:"product_name"` // 商品名称（查询时连接获取）
}

// ToVO converts Order to API OrderVO
func (o *Order) ToVO() *api.OrderVO {
	status := api.OrderStatus(o.Status)

	return &api.OrderVO{
		Id:          &o.ID,
		ProductId:   &o.ProductID,
		ActualPaid:  &o.ActualPaid,
		Status:      &status,
		WxPayNo:     o.WxPayNo,
		PayTime:     o.PayTime,
		CreatedAt:   &o.CreatedAt,
		ProductName: o.ProductName,
	}
}
