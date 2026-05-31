package models

import "time"

// EmailPromotionStatus 邮件推广状态
type EmailPromotionStatus int

// EmailPromotion 邮件推广记录
type EmailPromotion struct {
	ID            int                  `db:"id"`
	Channel       *string              `db:"channel"`
	BusinessTag   *string              `db:"business_tag"`
	TraceID       *string              `db:"trace_id"`
	OrderID       int                  `db:"order_id"`
	ProjectID     int                  `db:"project_id"`
	CreatorID     int                  `db:"creator_id"`
	Strategy      string               `db:"strategy"`
	MaxRecipients int                  `db:"max_recipients"` // 购买的最大发送人数
	TotalSent     int                  `db:"total_sent"`     // 实际发送数量
	Status        EmailPromotionStatus `db:"status"`         // 推广状态
	ErrorMessage  *string              `db:"error_message"`  // 错误信息
	StartedAt     *time.Time           `db:"started_at"`     // 开始发送时间
	CompletedAt   *time.Time           `db:"completed_at"`   // 完成时间
	CreatedAt     time.Time            `db:"created_at"`

	// Joined fields
	ProjectName *string  `db:"project_name"`
	Project     *Project `db:"-"`
}
