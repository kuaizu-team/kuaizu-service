package models

import "time"

type SmsNoticeStatus int

const (
	SmsNoticeStatusPending   SmsNoticeStatus = 0
	SmsNoticeStatusSending   SmsNoticeStatus = 1
	SmsNoticeStatusCompleted SmsNoticeStatus = 2
	SmsNoticeStatusFailed    SmsNoticeStatus = 3
)

type SmsNotice struct {
	ID                  int             `db:"id"`
	Channel             *string         `db:"channel"`
	BusinessTag         *string         `db:"business_tag"`
	TraceID             *string         `db:"trace_id"`
	OrderID             int             `db:"order_id"`
	OliveBranchRecordID int             `db:"olive_branch_record_id"`
	ProjectID           *int            `db:"project_id"`
	SenderID            int             `db:"sender_id"`
	ReceiverID          int             `db:"receiver_id"`
	SmsContent          string          `db:"sms_content"`
	Status              SmsNoticeStatus `db:"status"`
	ErrorMessage        *string         `db:"error_message"`
	Provider            *string         `db:"provider"`
	ProviderBizID       *string         `db:"provider_biz_id"`
	StartedAt           *time.Time      `db:"started_at"`
	CompletedAt         *time.Time      `db:"completed_at"`
	CreatedAt           time.Time       `db:"created_at"`
	UpdatedAt           time.Time       `db:"updated_at"`
}
