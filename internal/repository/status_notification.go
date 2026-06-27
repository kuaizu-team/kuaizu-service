package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

type StatusNotificationRepository struct{ db *sqlx.DB }

func NewStatusNotificationRepository(db *sqlx.DB) *StatusNotificationRepository {
	return &StatusNotificationRepository{db: db}
}

func CreateTx(ctx context.Context, tx *sqlx.Tx, userID, applicationID int, notificationType string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO status_notification(user_id, type, application_id) VALUES(?,?,?)`, userID, notificationType, applicationID)
	if err != nil {
		return fmt.Errorf("create status notification: %w", err)
	}
	return nil
}

func (r *StatusNotificationRepository) GetPending(ctx context.Context, userID int) (*models.StatusNotification, error) {
	const query = `SELECT sn.id, sn.user_id, sn.type, sn.application_id,
		pa.project_id, p.name AS project_name, pa.applied_at, pa.discussing_at,
		pa.rejected_at, pa.joined_at, rr.name AS reviewer_role_name,
		ar.name AS assigned_role_name, sn.created_at
		FROM status_notification sn
		JOIN project_application pa ON pa.id = sn.application_id
		JOIN project p ON p.id = pa.project_id
		LEFT JOIN project_role rr ON rr.code = pa.reviewer_role
		LEFT JOIN project_role ar ON ar.code = pa.assigned_role
		WHERE sn.user_id = ? AND sn.displayed_at IS NULL
		ORDER BY sn.id ASC LIMIT 1`
	var notification models.StatusNotification
	if err := r.db.GetContext(ctx, &notification, query, userID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get pending status notification: %w", err)
	}
	return &notification, nil
}

func (r *StatusNotificationRepository) MarkDisplayed(ctx context.Context, id int64, userID int) error {
	result, err := r.db.ExecContext(ctx, `UPDATE status_notification SET displayed_at=CURRENT_TIMESTAMP WHERE id=? AND user_id=? AND displayed_at IS NULL`, id, userID)
	if err != nil {
		return fmt.Errorf("mark status notification displayed: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}
