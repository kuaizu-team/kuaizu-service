package models

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/kuaizu-team/kuaizu-service/api"
)

const (
	OrderDeliverySceneEmailPromotion = "email_promotion"
	OrderDeliverySceneSMSNotice      = "sms_notice"
)

// OrderDeliveryIntent is persisted with an order so a paid callback can deliver it without client orchestration.
type OrderDeliveryIntent struct {
	Scene               string  `json:"scene"`
	ProjectID           *int    `json:"projectId,omitempty"`
	Strategy            string  `json:"strategy,omitempty"`
	ReceiverUserID      *int    `json:"receiverUserId,omitempty"`
	OliveBranchRecordID *int    `json:"oliveBranchRecordId,omitempty"`
	ApplicationID       *int    `json:"applicationId,omitempty"`
	MemberRemovalID     *int64  `json:"memberRemovalId,omitempty"`
	NoticeType          *string `json:"noticeType,omitempty"`
}

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
	PushStatus                *string    `db:"push_status"`
	PushRetryCount            int        `db:"push_retry_count"`
	LastPushTime              *time.Time `db:"last_push_time"`
	PushErrorMessage          *string    `db:"push_error_message"`
	DeliveryScene             *string    `db:"delivery_scene"`
	DeliveryPayload           *string    `db:"delivery_payload"`
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

// ParseDeliveryIntent returns nil for legacy orders that do not have a server-side delivery intent.
func (o *Order) ParseDeliveryIntent() (*OrderDeliveryIntent, error) {
	if o == nil || o.DeliveryScene == nil || o.DeliveryPayload == nil || *o.DeliveryScene == "" || *o.DeliveryPayload == "" {
		return nil, nil
	}
	var intent OrderDeliveryIntent
	if err := json.Unmarshal([]byte(*o.DeliveryPayload), &intent); err != nil {
		return nil, fmt.Errorf("parse order delivery intent: %w", err)
	}
	if intent.Scene == "" {
		intent.Scene = *o.DeliveryScene
	}
	if intent.Scene != *o.DeliveryScene {
		return nil, fmt.Errorf("order delivery scene does not match payload")
	}
	return &intent, nil
}

// ToVO converts Order to API OrderVO.
func (o *Order) ToVO() *api.OrderVO {
	status := api.OrderStatus(o.Status)
	var pushStatus *api.OrderVOPushStatus
	if o.PushStatus != nil {
		value := api.OrderVOPushStatus(*o.PushStatus)
		pushStatus = &value
	}
	var pushErrorMessage *string
	if o.PushStatus != nil && *o.PushStatus == "failed" && o.PushErrorMessage != nil {
		message := "消息发送失败，请稍后重试或申请退款"
		pushErrorMessage = &message
	}

	return &api.OrderVO{
		Id:                 &o.ID,
		ProductId:          &o.ProductID,
		TemplateCode:       o.TemplateCode,
		TemplateName:       o.TemplateName,
		ActualPaid:         &o.ActualPaid,
		Status:             &status,
		PushStatus:         pushStatus,
		PushRetryCount:     &o.PushRetryCount,
		LastPushTime:       o.LastPushTime,
		PushErrorMessage:   pushErrorMessage,
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
