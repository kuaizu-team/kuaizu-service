package models

import (
	"time"

	"github.com/kuaizu-team/kuaizu-service/api"
)

// MsgTemplateConfig 订阅消息模板配置
type MsgTemplateConfig struct {
	BizKey        string     `db:"biz_key"`
	TemplateID    string     `db:"template_id"`
	TemplateTitle string     `db:"template_title"`
	ContentJSON   string     `db:"content_json"` // 字段映射 JSON，例如 {"name": "thing1", "time": "time2"}
	CreatedAt     *time.Time `db:"created_at"`
	UpdatedAt     *time.Time `db:"updated_at"`
}

func (m *MsgTemplateConfig) ToVO() *api.MsgTemplateVO {
	return &api.MsgTemplateVO{
		BizKey:     m.BizKey,
		TemplateId: m.TemplateID,
	}
}
