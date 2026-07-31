package models

import (
	"time"

	"github.com/kuaizu-team/kuaizu-service/api"
)

// MsgTemplateConfig 订阅消息模板配置
type MsgTemplateConfig struct {
	BizKey             string     `db:"biz_key" json:"bizKey"`
	TemplateID         string     `db:"template_id" json:"templateId"`
	TemplateTitle      string     `db:"template_title" json:"templateTitle"`
	ContentJSON        string     `db:"content_json" json:"contentJson"`
	PagePath           *string    `db:"page_path" json:"pagePath"`
	Remark             *string    `db:"remark" json:"remark"`
	Enabled            bool       `db:"enabled" json:"enabled"`
	PlatformStatus     *string    `db:"platform_status" json:"platformStatus"`
	PlatformVerifiedAt *time.Time `db:"platform_verified_at" json:"platformVerifiedAt"`
	CreatedAt          *time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt          *time.Time `db:"updated_at" json:"updatedAt"`
}

func (m *MsgTemplateConfig) ToVO() *api.MsgTemplateVO {
	return &api.MsgTemplateVO{
		BizKey:     m.BizKey,
		TemplateId: m.TemplateID,
	}
}
