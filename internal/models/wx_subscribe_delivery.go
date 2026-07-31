package models

import "time"

const (
	WxSubscribeDeliveryPending    = "PENDING"
	WxSubscribeDeliveryProcessing = "PROCESSING"
	WxSubscribeDeliveryRetry      = "RETRY"
	WxSubscribeDeliverySent       = "SENT"
	WxSubscribeDeliverySkipped    = "SKIPPED"
	WxSubscribeDeliveryFailed     = "FAILED"
)

// WxSubscribeDelivery is both the durable outbox row and the delivery audit log.
type WxSubscribeDelivery struct {
	ID             int64      `db:"id" json:"id"`
	UserID         int        `db:"user_id" json:"userId"`
	BizKey         string     `db:"biz_key" json:"bizKey"`
	TemplateID     *string    `db:"template_id" json:"templateId"`
	BusinessData   string     `db:"business_data" json:"-"`
	PagePath       *string    `db:"page_path" json:"pagePath"`
	Status         string     `db:"status" json:"status"`
	AttemptCount   int        `db:"attempt_count" json:"attemptCount"`
	NextAttemptAt  time.Time  `db:"next_attempt_at" json:"nextAttemptAt"`
	ClaimedAt      *time.Time `db:"claimed_at" json:"claimedAt"`
	SentAt         *time.Time `db:"sent_at" json:"sentAt"`
	LastErrCode    *int       `db:"last_errcode" json:"lastErrcode"`
	LastErrMessage *string    `db:"last_errmsg" json:"lastErrmsg"`
	CreatedAt      time.Time  `db:"created_at" json:"createdAt"`
	UpdatedAt      time.Time  `db:"updated_at" json:"updatedAt"`
}
