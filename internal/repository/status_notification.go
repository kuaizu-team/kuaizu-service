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
	priority := statusNotificationPriority(notificationType)
	_, err := tx.ExecContext(ctx, `INSERT INTO status_notification(user_id, type, application_id, priority) VALUES(?,?,?,?)`, userID, notificationType, applicationID, priority)
	if err != nil {
		return fmt.Errorf("create status notification: %w", err)
	}
	return nil
}

func CreateOliveStatusNotificationTx(ctx context.Context, tx *sqlx.Tx, userID, oliveBranchID int, notificationType string) error {
	priority := statusNotificationPriority(notificationType)
	_, err := tx.ExecContext(ctx, `INSERT INTO status_notification(user_id, type, olive_branch_id, priority) VALUES(?,?,?,?)`, userID, notificationType, oliveBranchID, priority)
	if err != nil {
		return fmt.Errorf("create olive status notification: %w", err)
	}
	return nil
}

func CreateMemberRemovalStatusNotificationTx(ctx context.Context, tx *sqlx.Tx, userID int, removalID int64) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO status_notification(user_id,type,member_removal_id,priority) VALUES(?,?,?,1000)`, userID, models.StatusNotificationMemberRemoved, removalID)
	if err != nil {
		return fmt.Errorf("create member removal status notification: %w", err)
	}
	return nil
}

func statusNotificationPriority(notificationType string) int {
	if notificationType == models.StatusNotificationApplicationAccepted || notificationType == models.StatusNotificationOliveAccepted {
		return 100
	}
	return 10
}

func (r *StatusNotificationRepository) GetPending(ctx context.Context, userID int) (*models.StatusNotification, error) {
	const query = `SELECT sn.id, sn.user_id, sn.type, sn.application_id, sn.olive_branch_id, sn.member_removal_id, sn.priority,
		COALESCE(pa.project_id, ob.related_project_id, pmr.project_id) AS project_id,
		p.name AS project_name, COALESCE(pa.applied_at, ob.created_at, pmr.joined_at) AS applied_at,
		COALESCE(pa.discussing_at, ob.discussing_at) AS discussing_at,
		COALESCE(pa.rejected_at, ob.rejected_at) AS rejected_at,
		COALESCE(pa.joined_at, ob.admitted_at) AS joined_at,
		COALESCE(rr.name, obr.name) AS reviewer_role_name,
		COALESCE(ar.name, oar.name, pmrr.name) AS assigned_role_name,
		sender.nickname AS operator_name, pmr.removed_at, sn.created_at
		FROM status_notification sn
		LEFT JOIN project_application pa ON pa.id = sn.application_id
		LEFT JOIN olive_branch_record ob ON ob.id = sn.olive_branch_id
		LEFT JOIN project_member_removal pmr ON pmr.id = sn.member_removal_id
		JOIN project p ON p.id = COALESCE(pa.project_id, ob.related_project_id, pmr.project_id)
		LEFT JOIN project_role rr ON rr.code = pa.reviewer_role
		LEFT JOIN project_role ar ON ar.code = pa.assigned_role
		LEFT JOIN project_role obr ON obr.code = ob.operator_role
		LEFT JOIN project_role oar ON oar.code = ob.assigned_role
		LEFT JOIN project_role pmrr ON pmrr.code = pmr.role
		LEFT JOIN ` + "`user`" + ` sender ON sender.id = ob.sender_id
		WHERE sn.user_id = ? AND sn.displayed_at IS NULL
		ORDER BY sn.priority DESC, sn.created_at DESC, sn.id DESC LIMIT 1`
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
	result, err := r.db.ExecContext(ctx, `UPDATE status_notification SET displayed_at=CURRENT_TIMESTAMP WHERE user_id=? AND displayed_at IS NULL AND EXISTS (SELECT 1 FROM (SELECT id FROM status_notification WHERE id=? AND user_id=?) owned)`, userID, id, userID)
	if err != nil {
		return fmt.Errorf("mark status notification displayed: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}
