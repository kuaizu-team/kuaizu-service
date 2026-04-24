package repository

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

type MsgTemplateConfigRepository struct {
	db *sqlx.DB
}

func NewMsgTemplateConfigRepository(db *sqlx.DB) *MsgTemplateConfigRepository {
	return &MsgTemplateConfigRepository{db: db}
}

func (r *MsgTemplateConfigRepository) GetByBizKey(ctx context.Context, bizKey string) (*models.MsgTemplateConfig, error) {
	var config models.MsgTemplateConfig
	query := "SELECT biz_key, template_id, template_title, content_json, created_at, updated_at FROM msg_template_config WHERE biz_key = ?"
	err := r.db.GetContext(ctx, &config, query, bizKey)
	if err != nil {
		return nil, fmt.Errorf("get msg template config by biz_key: %w", err)
	}
	return &config, nil
}

func (r *MsgTemplateConfigRepository) GetByBizKeys(ctx context.Context, bizKeys []string) ([]models.MsgTemplateConfig, error) {
	if len(bizKeys) == 0 {
		return []models.MsgTemplateConfig{}, nil
	}

	query, args, err := sqlx.In("SELECT biz_key, template_id, template_title, content_json, created_at, updated_at FROM msg_template_config WHERE biz_key IN (?)", bizKeys)
	if err != nil {
		return nil, fmt.Errorf("build IN query: %w", err)
	}

	query = r.db.Rebind(query)
	var configs []models.MsgTemplateConfig
	err = r.db.SelectContext(ctx, &configs, query, args...)
	if err != nil {
		return nil, fmt.Errorf("select msg template configs by biz_keys: %w", err)
	}
	return configs, nil
}
