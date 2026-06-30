package models

import (
	"time"

	"github.com/kuaizu-team/kuaizu-service/api"
)

// Order represents an order in the database.
type Order struct {
	ID                        int        `db:"id"`
	UserID                    int        `db:"user_id"`
	ProductID                 int        `db:"product_id"`
	TemplateCode              *string    `db:"template_code"`
	TemplateName              *string    `db:"template_name"`
	Price                     float64    `db:"price"`
	Quantity                  int        `db:"quantity"`
	ActualPaid                float64    `db:"actual_paid"`
	Status                    int        `db:"status"`
	SettlementStatus          int        `db:"settlement_status"`
	RefundStatus              int        `db:"refund_status"`
	RefundReason              *string    `db:"refund_reason"`
	RefundApplyTime           *time.Time `db:"refund_apply_time"`
	RefundApplicantType       *int       `db:"refund_applicant_type"`
	RefundApplicantAdminID    *int       `db:"refund_applicant_admin_id"`
	RejectReason              *string    `db:"reject_reason"`
	RejectTime                *time.Time `db:"reject_time"`
	RefundWithdrawTime        *time.Time `db:"refund_withdraw_time"`
	RefundHandleTime          *time.Time `db:"refund_handle_time"`
	RefundOperatorAdminID     *int       `db:"refund_operator_admin_id"`
	SettlementBatchNo         *string    `db:"settlement_batch_no"`
	SettlementTime            *time.Time `db:"settlement_time"`
	SettlementOperatorAdminID *int       `db:"settlement_operator_admin_id"`
	WxPayNo                   *string    `db:"wx_pay_no"`
	OutTradeNo                *string    `db:"out_trade_no"`
	PayTime                   *time.Time `db:"pay_time"`
	CreatedAt                 time.Time  `db:"created_at"`
	UpdatedAt                 time.Time  `db:"updated_at"`

	// Joined fields from product table.
	ProductName        *string `db:"product_name"`
	ProductDescription *string `db:"product_description"`

	// Joined fields populated only in admin queries.
	UserNickname *string `db:"user_nickname"`
	SchoolName   *string `db:"school_name"`
	UserSchoolID *int    `db:"user_school_id"`
}

// ToVO converts Order to API OrderVO.
func (o *Order) ToVO() *api.OrderVO {
	status := api.OrderStatus(o.Status)

	return &api.OrderVO{
		Id:                 &o.ID,
		ProductId:          &o.ProductID,
		TemplateCode:       o.TemplateCode,
		TemplateName:       o.TemplateName,
		ActualPaid:         &o.ActualPaid,
		Status:             &status,
		RefundStatus:       &o.RefundStatus,
		RefundReason:       o.RefundReason,
		RefundApplyTime:    o.RefundApplyTime,
		RejectReason:       o.RejectReason,
		RejectTime:         o.RejectTime,
		RefundWithdrawTime: o.RefundWithdrawTime,
		WxPayNo:            o.WxPayNo,
		PayTime:            o.PayTime,
		CreatedAt:          &o.CreatedAt,
		ProductName:        o.ProductName,
	}
}
