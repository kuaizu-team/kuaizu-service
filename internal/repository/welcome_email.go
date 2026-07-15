package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

type WelcomeEmailDeliveryRepo interface {
	Create(ctx context.Context, userID int, email string) (int64, error)
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
	result, err := r.db.ExecContext(ctx, `
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
