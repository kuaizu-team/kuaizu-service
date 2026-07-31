package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

// SubscribeConfigRepository handles subscribe database operations
type SubscribeConfigRepository struct {
	db *sqlx.DB
}

// NewSubscribeConfigRepository creates a new SubscribeConfigRepository
func NewSubscribeConfigRepository(db *sqlx.DB) *SubscribeConfigRepository {
	return &SubscribeConfigRepository{db: db}
}

// GetByUserIDAndBizKey retrieves a subscribe config by user_id and biz_key
func (r *SubscribeConfigRepository) GetByUserIDAndBizKey(ctx context.Context, userID int, bizKey string) (*models.SubscribeConfig, error) {
	query := `
		SELECT id, user_id, biz_key, status, created_at, updated_at
		FROM subscribe
		WHERE user_id = ? AND biz_key = ?
	`

	var config models.SubscribeConfig
	if err := r.db.QueryRowxContext(ctx, query, userID, bizKey).StructScan(&config); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query subscribe config by biz_key: %w", err)
	}

	return &config, nil
}

// ListByUserID retrieves all subscribe configs for a user
func (r *SubscribeConfigRepository) ListByUserID(ctx context.Context, userID int) ([]models.SubscribeConfig, error) {
	query := `
		SELECT id, user_id, biz_key, status, created_at, updated_at
		FROM subscribe
		WHERE user_id = ?
		ORDER BY created_at DESC
	`

	var configs []models.SubscribeConfig
	if err := r.db.SelectContext(ctx, &configs, query, userID); err != nil {
		return nil, fmt.Errorf("query subscribe configs: %w", err)
	}

	return configs, nil
}

func (r *SubscribeConfigRepository) ListAcceptedUserIDsByBizKey(ctx context.Context, bizKey string, limit int, afterUserID int) ([]int, error) {
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	if afterUserID < 0 {
		afterUserID = 0
	}
	query := `
		SELECT user_id
		FROM subscribe
		WHERE biz_key = ? AND status IN (?, ?) AND user_id > ?
		ORDER BY user_id ASC
		LIMIT ?
	`
	var ids []int
	if err := r.db.SelectContext(ctx, &ids, query, bizKey, models.SubscribeStatusAccept, models.SubscribeStatusAlways, afterUserID, limit); err != nil {
		return nil, fmt.Errorf("list accepted subscribe users: %w", err)
	}
	return ids, nil
}

func (r *SubscribeConfigRepository) UpsertWithHistory(ctx context.Context, config *models.SubscribeConfig, templateID, result, source string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin subscribe status transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.NamedExecContext(ctx, `
		INSERT INTO subscribe (user_id, biz_key, status)
		VALUES (:user_id, :biz_key, :status)
		ON DUPLICATE KEY UPDATE
			status = VALUES(status),
			updated_at = CURRENT_TIMESTAMP
	`, config); err != nil {
		return fmt.Errorf("upsert subscribe config: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO wx_subscribe_status_history
			(user_id, biz_key, template_id, result, status, source)
		VALUES (?, ?, ?, ?, ?, ?)
	`, config.UserID, config.BizKey, templateID, result, config.Status, source); err != nil {
		return fmt.Errorf("record subscribe status history: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit subscribe status transaction: %w", err)
	}
	return nil
}

// UpdateStatus updates the status of a subscribe config
func (r *SubscribeConfigRepository) UpdateStatus(ctx context.Context, userID int, bizKey string, status models.SubscribeStatus) error {
	query := `
		UPDATE subscribe
		SET status = ?, updated_at = CURRENT_TIMESTAMP
		WHERE user_id = ? AND biz_key = ?
	`

	_, err := r.db.ExecContext(ctx, query, status, userID, bizKey)
	if err != nil {
		return fmt.Errorf("update subscribe config status: %w", err)
	}

	return nil
}
