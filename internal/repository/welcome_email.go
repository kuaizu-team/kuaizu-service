package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

type PendingWelcomeEmailDelivery struct {
	ID       int64  `db:"id"`
	UserID   int    `db:"user_id"`
	Email    string `db:"recipient_email"`
	Nickname string `db:"nickname"`
}

type WelcomeEmailDeliveryRepo interface {
	Create(ctx context.Context, userID int, email string) (int64, error)
	ListPendingBefore(ctx context.Context, before time.Time, limit int) ([]PendingWelcomeEmailDelivery, error)
	ClaimPending(ctx context.Context, deliveryID int64, staleBefore time.Time) (bool, error)
	MarkSent(ctx context.Context, deliveryID int64, taskID *int64) error
	MarkFailed(ctx context.Context, deliveryID int64, errorMessage string) error
}

type WelcomeEmailDeliveryRepository struct {
	db *sqlx.DB
}

func NewWelcomeEmailDeliveryRepository(db *sqlx.DB) *WelcomeEmailDeliveryRepository {
	return &WelcomeEmailDeliveryRepository{db: db}
}

// Create records every genuine email change as an independent delivery.
// Historical deliveries to the same address never suppress a new one.
func (r *WelcomeEmailDeliveryRepository) Create(ctx context.Context, userID int, email string) (int64, error) {
	return createWelcomeEmailDelivery(ctx, r.db, userID, email)
}

// CreateWelcomeEmailDeliveryTx records the delivery in the same transaction as
// the user email change, so concurrent identical updates cannot create duplicates.
func CreateWelcomeEmailDeliveryTx(ctx context.Context, tx *sqlx.Tx, userID int, email string) (int64, error) {
	return createWelcomeEmailDelivery(ctx, tx, userID, email)
}

func createWelcomeEmailDelivery(ctx context.Context, exec sqlx.ExtContext, userID int, email string) (int64, error) {
	result, err := exec.ExecContext(ctx, `
		INSERT INTO welcome_email_delivery
			(recipient_email, user_id, status, created_at, updated_at)
		VALUES (?, ?, 'pending', NOW(), NOW())`, strings.ToLower(strings.TrimSpace(email)), userID)
	if err != nil {
		return 0, fmt.Errorf("create welcome email delivery: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get welcome email delivery id: %w", err)
	}
	return id, nil
}
func (r *WelcomeEmailDeliveryRepository) ListPendingBefore(ctx context.Context, before time.Time, limit int) ([]PendingWelcomeEmailDelivery, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	query := `
		SELECT d.id,
		       COALESCE(d.user_id, 0) AS user_id,
		       d.recipient_email,
		       COALESCE(NULLIF(TRIM(u.nickname), ''), '同学') AS nickname
		FROM welcome_email_delivery d
		LEFT JOIN ` + "`user`" + ` u ON u.id = d.user_id
		WHERE d.status = 'pending' AND d.updated_at <= ?
		ORDER BY d.id
		LIMIT ?`
	var deliveries []PendingWelcomeEmailDelivery
	if err := r.db.SelectContext(ctx, &deliveries, query, before, limit); err != nil {
		return nil, fmt.Errorf("list pending welcome emails: %w", err)
	}
	return deliveries, nil
}

func (r *WelcomeEmailDeliveryRepository) ClaimPending(ctx context.Context, deliveryID int64, staleBefore time.Time) (bool, error) {
	result, err := r.db.ExecContext(ctx, `
		UPDATE welcome_email_delivery
		SET updated_at = NOW()
		WHERE id = ? AND status = 'pending' AND updated_at <= ?`, deliveryID, staleBefore)
	if err != nil {
		return false, fmt.Errorf("claim pending welcome email: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read claimed welcome email rows: %w", err)
	}
	return rows == 1, nil
}

func (r *WelcomeEmailDeliveryRepository) MarkSent(ctx context.Context, deliveryID int64, taskID *int64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE welcome_email_delivery
		SET status = 'sent', message_task_id = ?, sent_at = ?, error_message = NULL, updated_at = NOW()
		WHERE id = ? AND status = 'pending'`, taskID, time.Now(), deliveryID)
	if err != nil {
		return fmt.Errorf("mark welcome email sent: %w", err)
	}
	return nil
}

func (r *WelcomeEmailDeliveryRepository) MarkFailed(ctx context.Context, deliveryID int64, errorMessage string) error {
	if len(errorMessage) > 500 {
		errorMessage = errorMessage[:500]
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE welcome_email_delivery
		SET status = 'failed', error_message = ?, updated_at = NOW()
		WHERE id = ? AND status = 'pending'`, errorMessage, deliveryID)
	if err != nil {
		return fmt.Errorf("mark welcome email failed: %w", err)
	}
	return nil
}
