package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

const maxOrderPushRetries = 3

// BeginOrderPushDeliveryForUser atomically claims a paid order for one delivery attempt.
func (r *Repository) BeginOrderPushDeliveryForUser(ctx context.Context, id, userID int) (bool, error) {
	if r == nil || r.db == nil {
		return true, nil
	}
	result, err := r.db.ExecContext(ctx, `UPDATE `+"`order`"+` SET
		push_status='pending', push_error_message=NULL, last_push_time=NOW(), updated_at=NOW()
		WHERE id=? AND user_id=? AND status=? AND refund_status=0
		  AND push_status IS NULL`,
		id, userID, models.OrderStatusPaid)
	if err != nil {
		return false, fmt.Errorf("begin owned order delivery: %w", err)
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}

// ListRecoverableOrderDeliveries finds committed delivery intents that are unclaimed or stale pending.
// Failed deliveries require the bounded, explicit retry flow.
func (r *Repository) ListRecoverableOrderDeliveries(ctx context.Context, staleBefore time.Time, limit int) ([]*models.Order, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	var orders []*models.Order
	err := r.db.SelectContext(ctx, &orders, `SELECT
		id, user_id, product_id, quantity, status, push_status, refund_status,
		delivery_scene, delivery_payload, updated_at
		FROM `+"`order`"+`
		WHERE status=? AND refund_status=0
		  AND delivery_scene IS NOT NULL AND delivery_payload IS NOT NULL
		  AND (push_status IS NULL OR (push_status='pending' AND updated_at < ?))
		ORDER BY updated_at ASC, id ASC
		LIMIT ?`, models.OrderStatusPaid, staleBefore, limit)
	if err != nil {
		return nil, fmt.Errorf("list recoverable order deliveries: %w", err)
	}
	return orders, nil
}

// ClaimRecoverableOrderDelivery atomically takes one recovery candidate across service instances.
func (r *Repository) ClaimRecoverableOrderDelivery(ctx context.Context, id int, staleBefore time.Time) (bool, error) {
	if r == nil || r.db == nil {
		return true, nil
	}
	result, err := r.db.ExecContext(ctx, `UPDATE `+"`order`"+` SET
		push_status='pending', push_error_message=NULL, last_push_time=NOW(), updated_at=NOW()
		WHERE id=? AND status=? AND refund_status=0
		  AND delivery_scene IS NOT NULL AND delivery_payload IS NOT NULL
		  AND (push_status IS NULL OR (push_status='pending' AND updated_at < ?))`,
		id, models.OrderStatusPaid, staleBefore)
	if err != nil {
		return false, fmt.Errorf("claim recoverable order delivery: %w", err)
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}

// UpdateOrderPushStatus updates order delivery state. Nil DB is tolerated by unit-test repositories.
func (r *Repository) UpdateOrderPushStatus(ctx context.Context, id int, status string, errorMessage *string) error {
	if r == nil || r.db == nil {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `UPDATE `+"`order`"+` SET
		push_status=?, push_error_message=?, last_push_time=NOW(), updated_at=NOW()
		WHERE id=? AND (push_status IS NULL OR push_status <> 'success' OR ? = 'success')`,
		status, errorMessage, id, status)
	if err != nil {
		return fmt.Errorf("update order push status: %w", err)
	}
	return nil
}

// UpdateOrderPushStatusForUser updates delivery state only when the order belongs to the caller.
// The ownership predicate is a defense-in-depth guard for user-triggered send flows.
func (r *Repository) UpdateOrderPushStatusForUser(ctx context.Context, id, userID int, status string, errorMessage *string) (bool, error) {
	if r == nil || r.db == nil {
		return true, nil
	}
	result, err := r.db.ExecContext(ctx, `UPDATE `+"`order`"+` SET
		push_status=?, push_error_message=?, last_push_time=NOW(), updated_at=NOW()
		WHERE id=? AND user_id=?
		  AND (push_status IS NULL OR push_status <> 'success' OR ? = 'success')`,
		status, errorMessage, id, userID, status)
	if err != nil {
		return false, fmt.Errorf("update owned order push status: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read owned order push status result: %w", err)
	}
	return affected == 1, nil
}

// BeginOrderPushRetry atomically changes a failed push to pending and increments its retry counter.
func (r *Repository) BeginOrderPushRetry(ctx context.Context, id int) (bool, error) {
	if r == nil || r.db == nil {
		return false, nil
	}
	result, err := r.db.ExecContext(ctx, `UPDATE `+"`order`"+` SET
		push_status='pending', push_retry_count=push_retry_count+1,
		push_error_message=NULL, last_push_time=NOW(), updated_at=NOW()
		WHERE id=? AND push_status='failed' AND refund_status=0
		  AND push_retry_count < ?`, id, maxOrderPushRetries)
	if err != nil {
		return false, fmt.Errorf("begin order push retry: %w", err)
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}
