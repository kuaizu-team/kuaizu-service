package models

import "time"

// SubscribeStatus 订阅状态
type SubscribeStatus int

const (
	SubscribeStatusAccept SubscribeStatus = 0 // 允许
	SubscribeStatusReject SubscribeStatus = 1 // 拒绝
	SubscribeStatusAlways SubscribeStatus = 2 // 总是保持
)

// SubscribeConfig 订阅消息配置
type SubscribeConfig struct {
	ID        int             `db:"id"`
	UserID    int             `db:"user_id"`
	BizKey    string          `db:"biz_key"`
	Status    SubscribeStatus `db:"status"`
	CreatedAt *time.Time      `db:"created_at"`
	UpdatedAt *time.Time      `db:"updated_at"`
}
