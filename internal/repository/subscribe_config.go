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
		SELECT id, user_id, biz_key, subscribe_count, status, created_at, updated_at
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
		SELECT id, user_id, biz_key, subscribe_count, status, created_at, updated_at
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

// Upsert creates or updates a subscribe config
func (r *SubscribeConfigRepository) Upsert(ctx context.Context, config *models.SubscribeConfig) error {
	query := `
		INSERT INTO subscribe (user_id, biz_key, subscribe_count, status)
		VALUES (:user_id, :biz_key, :subscribe_count, :status)
		ON DUPLICATE KEY UPDATE
			subscribe_count = VALUES(subscribe_count),
			status = VALUES(status),
			updated_at = CURRENT_TIMESTAMP
	`

	_, err := r.db.NamedExecContext(ctx, query, config)
	if err != nil {
		return fmt.Errorf("upsert subscribe config: %w", err)
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

// DecrementCount decrements the subscribe_count by 1
func (r *SubscribeConfigRepository) DecrementCount(ctx context.Context, userID int, bizKey string) error {
	query := `
		UPDATE subscribe
		SET subscribe_count = GREATEST(0, subscribe_count - 1),
		    updated_at = CURRENT_TIMESTAMP
		WHERE user_id = ? AND biz_key = ? AND subscribe_count > 0
	`

	_, err := r.db.ExecContext(ctx, query, userID, bizKey)
	if err != nil {
		return fmt.Errorf("decrement subscribe count: %w", err)
	}

	return nil
}

// IncrementCount increments the subscribe_count by specified amount
func (r *SubscribeConfigRepository) IncrementCount(ctx context.Context, userID int, bizKey string, count int) error {
	query := `
		UPDATE subscribe
		SET subscribe_count = subscribe_count + ?,
		    updated_at = CURRENT_TIMESTAMP
		WHERE user_id = ? AND biz_key = ?
	`

	_, err := r.db.ExecContext(ctx, query, count, userID, bizKey)
	if err != nil {
		return fmt.Errorf("increment subscribe count: %w", err)
	}

	return nil
}
