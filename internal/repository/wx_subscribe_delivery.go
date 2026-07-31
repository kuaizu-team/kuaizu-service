package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

type WxSubscribeDeliveryRepository struct {
	db *sqlx.DB
}

func NewWxSubscribeDeliveryRepository(db *sqlx.DB) *WxSubscribeDeliveryRepository {
	return &WxSubscribeDeliveryRepository{db: db}
}

func (r *WxSubscribeDeliveryRepository) CheckSchema(ctx context.Context) error {
	var value int
	for _, tableName := range []string{"wx_subscribe_delivery", "wx_subscribe_status_history"} {
		if err := r.db.GetContext(ctx, &value, `
			SELECT COUNT(*) FROM information_schema.tables
			WHERE table_schema = DATABASE() AND table_name = ?
		`, tableName); err != nil {
			return fmt.Errorf("check %s schema: %w", tableName, err)
		}
		if value != 1 {
			return fmt.Errorf("%s migration is required", tableName)
		}
	}
	if err := r.db.GetContext(ctx, &value, `
		SELECT COUNT(*) FROM (
			SELECT enabled, platform_status, platform_verified_at, remark
			FROM msg_template_config LIMIT 1
		) AS schema_check
	`); err != nil {
		return fmt.Errorf("msg_template_config management columns migration is required: %w", err)
	}
	if err := r.db.GetContext(ctx, &value, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = 'msg_template_config'
			AND column_name = 'remark' AND character_maximum_length = 20
	`); err != nil {
		return fmt.Errorf("check msg_template_config.remark definition: %w", err)
	}
	if value != 1 {
		return fmt.Errorf("msg_template_config.remark must be varchar(20)")
	}
	return nil
}

func (r *WxSubscribeDeliveryRepository) Create(ctx context.Context, delivery *models.WxSubscribeDelivery) (int64, error) {
	result, err := r.db.NamedExecContext(ctx, `
		INSERT INTO wx_subscribe_delivery
			(user_id, biz_key, business_data, page_path, status, next_attempt_at)
		VALUES
			(:user_id, :biz_key, :business_data, :page_path, :status, CURRENT_TIMESTAMP)
	`, delivery)
	if err != nil {
		return 0, fmt.Errorf("create wx subscribe delivery: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get wx subscribe delivery id: %w", err)
	}
	return id, nil
}

func (r *WxSubscribeDeliveryRepository) GetByID(ctx context.Context, id int64) (*models.WxSubscribeDelivery, error) {
	var delivery models.WxSubscribeDelivery
	err := r.db.GetContext(ctx, &delivery, `
		SELECT id, user_id, biz_key, template_id, business_data, page_path,
			status, attempt_count, next_attempt_at, claimed_at, sent_at,
			last_errcode, last_errmsg, created_at, updated_at
		FROM wx_subscribe_delivery WHERE id = ?
	`, id)
	if err != nil {
		return nil, fmt.Errorf("get wx subscribe delivery: %w", err)
	}
	return &delivery, nil
}

func (r *WxSubscribeDeliveryRepository) ListDue(ctx context.Context, staleBefore time.Time, limit int) ([]int64, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var ids []int64
	err := r.db.SelectContext(ctx, &ids, `
		SELECT id FROM wx_subscribe_delivery
		WHERE (
			status IN (?, ?) AND next_attempt_at <= CURRENT_TIMESTAMP
		) OR (
			status = ? AND claimed_at < ?
		)
		ORDER BY id ASC LIMIT ?
	`, models.WxSubscribeDeliveryPending, models.WxSubscribeDeliveryRetry,
		models.WxSubscribeDeliveryProcessing, staleBefore, limit)
	if err != nil {
		return nil, fmt.Errorf("list due wx subscribe deliveries: %w", err)
	}
	return ids, nil
}

func (r *WxSubscribeDeliveryRepository) Claim(ctx context.Context, id int64, staleBefore time.Time) (bool, error) {
	result, err := r.db.ExecContext(ctx, `
		UPDATE wx_subscribe_delivery
		SET status = ?, attempt_count = attempt_count + 1,
			claimed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND (
			(status IN (?, ?) AND next_attempt_at <= CURRENT_TIMESTAMP)
			OR (status = ? AND claimed_at < ?)
		)
	`, models.WxSubscribeDeliveryProcessing, id,
		models.WxSubscribeDeliveryPending, models.WxSubscribeDeliveryRetry,
		models.WxSubscribeDeliveryProcessing, staleBefore)
	if err != nil {
		return false, fmt.Errorf("claim wx subscribe delivery: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected == 1, nil
}

func (r *WxSubscribeDeliveryRepository) MarkSent(ctx context.Context, id int64, templateID string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE wx_subscribe_delivery
		SET status = ?, template_id = ?, sent_at = CURRENT_TIMESTAMP,
			last_errcode = NULL, last_errmsg = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, models.WxSubscribeDeliverySent, templateID, id)
	return err
}

func (r *WxSubscribeDeliveryRepository) MarkSkipped(ctx context.Context, id int64, templateID string, errCode int, message string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE wx_subscribe_delivery
		SET status = ?, template_id = ?, last_errcode = ?, last_errmsg = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, models.WxSubscribeDeliverySkipped, templateID, errCode, message, id)
	return err
}

func (r *WxSubscribeDeliveryRepository) MarkFailed(ctx context.Context, id int64, templateID string, errCode *int, message string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE wx_subscribe_delivery
		SET status = ?, template_id = NULLIF(?, ''), last_errcode = ?, last_errmsg = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, models.WxSubscribeDeliveryFailed, templateID, errCode, message, id)
	return err
}

func (r *WxSubscribeDeliveryRepository) ScheduleRetry(ctx context.Context, id int64, templateID string, errCode *int, message string, nextAttemptAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE wx_subscribe_delivery
		SET status = ?, template_id = NULLIF(?, ''), last_errcode = ?, last_errmsg = ?,
			next_attempt_at = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, models.WxSubscribeDeliveryRetry, templateID, errCode, message, nextAttemptAt, id)
	return err
}

func (r *WxSubscribeDeliveryRepository) ListRecent(ctx context.Context, limit int) ([]models.WxSubscribeDelivery, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	deliveries := make([]models.WxSubscribeDelivery, 0)
	err := r.db.SelectContext(ctx, &deliveries, `
		SELECT id, user_id, biz_key, template_id, business_data, page_path,
			status, attempt_count, next_attempt_at, claimed_at, sent_at,
			last_errcode, last_errmsg, created_at, updated_at
		FROM wx_subscribe_delivery ORDER BY id DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list recent wx subscribe deliveries: %w", err)
	}
	return deliveries, nil
}

func (r *WxSubscribeDeliveryRepository) CountByStatusSince(ctx context.Context, since time.Time) (map[string]int, error) {
	type statusCount struct {
		Status string `db:"status"`
		Count  int    `db:"count"`
	}
	var rows []statusCount
	if err := r.db.SelectContext(ctx, &rows, `
		SELECT status, COUNT(*) AS count FROM wx_subscribe_delivery
		WHERE created_at >= ? GROUP BY status
	`, since); err != nil {
		return nil, fmt.Errorf("count wx subscribe deliveries: %w", err)
	}
	result := make(map[string]int, len(rows))
	for _, row := range rows {
		result[row.Status] = row.Count
	}
	return result, nil
}
