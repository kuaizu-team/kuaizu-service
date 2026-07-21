package repository

import (
	"context"
	"fmt"
)

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

// BeginOrderPushRetry atomically changes a failed push to pending and increments its retry counter.
func (r *Repository) BeginOrderPushRetry(ctx context.Context, id int) (bool, error) {
	if r == nil || r.db == nil {
		return false, nil
	}
	result, err := r.db.ExecContext(ctx, `UPDATE `+"`order`"+` SET
		push_status='pending', push_retry_count=push_retry_count+1,
		push_error_message=NULL, last_push_time=NOW(), updated_at=NOW()
		WHERE id=? AND push_status='failed' AND refund_status=0`, id)
	if err != nil {
		return false, fmt.Errorf("begin order push retry: %w", err)
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}
