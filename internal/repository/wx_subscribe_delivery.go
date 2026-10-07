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
	return createWxSubscribeDelivery(ctx, r.db, delivery)
}
func CreateWxSubscribeDeliveryTx(ctx context.Context, tx *sqlx.Tx, delivery *models.WxSubscribeDelivery) (int64, error) {
	return createWxSubscribeDelivery(ctx, tx, delivery)
}
func createWxSubscribeDelivery(ctx context.Context, exec sqlx.ExtContext, delivery *models.WxSubscribeDelivery) (int64, error) {
	result, err := sqlx.NamedExecContext(ctx, exec, `
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
	// Stale provider dispatch must be reconciled, never lease-retried.
	if _, err := r.db.ExecContext(ctx, `UPDATE wx_subscribe_delivery
      SET status=?,last_errmsg='Dispatch outcome unknown; reconcile before retry',updated_at=CURRENT_TIMESTAMP
      WHERE status IN (?,?) AND claimed_at<?`, models.WxSubscribeDeliveryUnknown, models.WxSubscribeDeliveryDispatching, models.WxSubscribeDeliveryProcessing, staleBefore); err != nil {
		return nil, err
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
		models.WxSubscribeDeliveryPreparing, staleBefore, limit)
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
	`, models.WxSubscribeDeliveryPreparing, id,
		models.WxSubscribeDeliveryPending, models.WxSubscribeDeliveryRetry,
		models.WxSubscribeDeliveryPreparing, staleBefore)
	if err != nil {
		return false, fmt.Errorf("claim wx subscribe delivery: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected == 1, nil
}

func (r *WxSubscribeDeliveryRepository) BeginDispatch(ctx context.Context, id int64, attempt int) (bool, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE wx_subscribe_delivery SET status=?,claimed_at=CURRENT_TIMESTAMP
 WHERE id=? AND attempt_count=? AND status=?`, models.WxSubscribeDeliveryDispatching, id, attempt, models.WxSubscribeDeliveryPreparing)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}
func (r *WxSubscribeDeliveryRepository) MarkUnknown(ctx context.Context, id int64, attempt int, templateID, message string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE wx_subscribe_delivery SET status=?,template_id=NULLIF(?,''),last_errmsg=?,updated_at=CURRENT_TIMESTAMP
 WHERE id=? AND attempt_count=? AND status=?`, models.WxSubscribeDeliveryUnknown, templateID, message, id, attempt, models.WxSubscribeDeliveryDispatching)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("wx subscribe claim lost for id=%d attempt=%d", id, attempt)
	}
	return nil
}

func (r *WxSubscribeDeliveryRepository) MarkSent(ctx context.Context, id int64, attempt int, templateID string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE wx_subscribe_delivery
		SET status = ?, template_id = ?, sent_at = CURRENT_TIMESTAMP,
			last_errcode = NULL, last_errmsg = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND attempt_count = ? AND status IN ('PREPARING','DISPATCHING')
	`, models.WxSubscribeDeliverySent, templateID, id, attempt)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("wx subscribe claim lost for id=%d attempt=%d", id, attempt)
	}
	return nil
}

func (r *WxSubscribeDeliveryRepository) MarkSkipped(ctx context.Context, id int64, attempt int, templateID string, errCode int, message string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE wx_subscribe_delivery
		SET status = ?, template_id = ?, last_errcode = ?, last_errmsg = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND attempt_count = ? AND status IN ('PREPARING','DISPATCHING')
	`, models.WxSubscribeDeliverySkipped, templateID, errCode, message, id, attempt)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("wx subscribe claim lost for id=%d attempt=%d", id, attempt)
	}
	return nil
}

func (r *WxSubscribeDeliveryRepository) MarkFailed(ctx context.Context, id int64, attempt int, templateID string, errCode *int, message string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE wx_subscribe_delivery
		SET status = ?, template_id = NULLIF(?, ''), last_errcode = ?, last_errmsg = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND attempt_count = ? AND status IN ('PREPARING','DISPATCHING')
	`, models.WxSubscribeDeliveryFailed, templateID, errCode, message, id, attempt)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("wx subscribe claim lost for id=%d attempt=%d", id, attempt)
	}
	return nil
}

func (r *WxSubscribeDeliveryRepository) ScheduleRetry(ctx context.Context, id int64, attempt int, templateID string, errCode *int, message string, nextAttemptAt time.Time) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE wx_subscribe_delivery
		SET status = ?, template_id = NULLIF(?, ''), last_errcode = ?, last_errmsg = ?,
			next_attempt_at = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND attempt_count = ? AND status IN ('PREPARING','DISPATCHING')
	`, models.WxSubscribeDeliveryRetry, templateID, errCode, message, nextAttemptAt, id, attempt)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("wx subscribe claim lost for id=%d attempt=%d", id, attempt)
	}
	return nil
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
