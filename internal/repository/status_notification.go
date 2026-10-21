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
	result, err := tx.ExecContext(ctx, `INSERT INTO status_notification(user_id, type, application_id, priority) VALUES(?,?,?,?)`, userID, notificationType, applicationID, priority)
	if err != nil {
		return fmt.Errorf("create status notification: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	return archiveStatusNotificationTx(ctx, tx, id)
}

func CreateOliveStatusNotificationTx(ctx context.Context, tx *sqlx.Tx, userID, oliveBranchID int, notificationType string) error {
	priority := statusNotificationPriority(notificationType)
	result, err := tx.ExecContext(ctx, `INSERT INTO status_notification(user_id, type, olive_branch_id, priority) VALUES(?,?,?,?)`, userID, notificationType, oliveBranchID, priority)
	if err != nil {
		return fmt.Errorf("create olive status notification: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	return archiveStatusNotificationTx(ctx, tx, id)
}

func CreateMemberRemovalStatusNotificationTx(ctx context.Context, tx *sqlx.Tx, userID int, removalID int64) error {
	result, err := tx.ExecContext(ctx, `INSERT INTO status_notification(user_id,type,member_removal_id,priority) VALUES(?,?,?,1000)`, userID, models.StatusNotificationMemberRemoved, removalID)
	if err != nil {
		return fmt.Errorf("create member removal status notification: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	return archiveStatusNotificationTx(ctx, tx, id)
}

func statusNotificationPriority(notificationType string) int {
	if notificationType == models.StatusNotificationApplicationAccepted || notificationType == models.StatusNotificationOliveAccepted {
		return 100
	}
	return 10
}

func (r *StatusNotificationRepository) GetPending(ctx context.Context, userID int) (*models.StatusNotification, error) {
	const query = `SELECT sn.id, sn.user_id, sn.type, sn.application_id, sn.olive_branch_id, sn.member_removal_id, sn.priority, sm.id AS message_id,
		COALESCE(pa.project_id, ob.related_project_id, pmr.project_id) AS project_id,
		p.name AS project_name, COALESCE(pa.applied_at, ob.created_at, pmr.joined_at) AS applied_at,
		COALESCE(pa.discussing_at, ob.discussing_at) AS discussing_at,
		COALESCE(pa.rejected_at, ob.rejected_at) AS rejected_at,
		COALESCE(pa.joined_at, ob.admitted_at) AS joined_at,
		COALESCE(rr.name, obr.name) AS reviewer_role_name,
		COALESCE(ar.name, oar.name, pmrr.name) AS assigned_role_name,
		sender.nickname AS operator_name, pmr.removed_at, sn.created_at
		FROM status_notification sn
		LEFT JOIN user_status_message sm ON sm.status_notification_id=sn.id AND sm.user_id=sn.user_id
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
	if _, err := r.db.ExecContext(ctx, `UPDATE status_notification SET displayed_at=CURRENT_TIMESTAMP WHERE user_id=? AND displayed_at IS NULL AND id<>? AND type NOT IN ('cooperation-approved','cooperation-rejected')`, userID, notification.ID); err != nil {
		return nil, fmt.Errorf("mark lower priority status notifications displayed: %w", err)
	}
	return &notification, nil
}

func (r *StatusNotificationRepository) MarkDisplayed(ctx context.Context, id int64, userID int) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE status_notification SET displayed_at=CURRENT_TIMESTAMP WHERE id=? AND user_id=? AND displayed_at IS NULL`, id, userID)
	if err != nil {
		return fmt.Errorf("mark status notification displayed: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	if _, err := tx.ExecContext(ctx, `UPDATE user_status_message SET read_at=COALESCE(read_at,CURRENT_TIMESTAMP), fullscreen_displayed=1, fullscreen_displayed_at=COALESCE(fullscreen_displayed_at,CURRENT_TIMESTAMP) WHERE status_notification_id=? AND user_id=?`, id, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *StatusNotificationRepository) MarkAllPendingDisplayed(ctx context.Context, userID int) error {
	if _, err := r.db.ExecContext(ctx, `UPDATE status_notification SET displayed_at=CURRENT_TIMESTAMP WHERE user_id=? AND displayed_at IS NULL`, userID); err != nil {
		return fmt.Errorf("mark all pending status notifications displayed: %w", err)
	}
	return nil
}

// Only clients opting in to cooperation can select/suppress these new types.
// Relative priorities and the separate administrator-invitation gate are unchanged.
func (r *StatusNotificationRepository) GetPendingIncludingCooperation(ctx context.Context, userID int) (*models.StatusNotification, error) {
	const query = `SELECT sn.id,sn.user_id,sn.type,sn.application_id,sn.olive_branch_id,sn.member_removal_id,sn.priority,sm.id AS message_id,
 COALESCE(pa.project_id,ob.related_project_id,pmr.project_id,0) AS project_id,
 COALESCE(p.name,'') AS project_name,COALESCE(pa.applied_at,ob.created_at,pmr.joined_at,ca.applied_at) AS applied_at,
 COALESCE(pa.discussing_at,ob.discussing_at) AS discussing_at,COALESCE(pa.rejected_at,ob.rejected_at) AS rejected_at,
 COALESCE(pa.joined_at,ob.admitted_at) AS joined_at,COALESCE(rr.name,obr.name) AS reviewer_role_name,
 COALESCE(ar.name,oar.name,pmrr.name) AS assigned_role_name,sender.nickname AS operator_name,pmr.removed_at,sn.created_at,
 ca.id AS cooperation_application_id,ca.cooperation_id,COALESCE(JSON_UNQUOTE(JSON_EXTRACT(sm.payload,'$.cooperationTitle')),c.title) AS cooperation_title,
 COALESCE(JSON_UNQUOTE(JSON_EXTRACT(sm.payload,'$.companyName')),co.company_name) AS company_name,
 COALESCE(JSON_UNQUOTE(JSON_EXTRACT(sm.payload,'$.schoolName')),school.school_name) AS school_name,
 ca.status AS application_status,ca.reviewed_at
 FROM status_notification sn
 LEFT JOIN user_status_message sm ON sm.status_notification_id=sn.id AND sm.user_id=sn.user_id
 LEFT JOIN project_application pa ON pa.id=sn.application_id
 LEFT JOIN olive_branch_record ob ON ob.id=sn.olive_branch_id
 LEFT JOIN project_member_removal pmr ON pmr.id=sn.member_removal_id
 LEFT JOIN project p ON p.id=COALESCE(pa.project_id,ob.related_project_id,pmr.project_id)
 LEFT JOIN project_role rr ON rr.code=pa.reviewer_role LEFT JOIN project_role ar ON ar.code=pa.assigned_role
 LEFT JOIN project_role obr ON obr.code=ob.operator_role LEFT JOIN project_role oar ON oar.code=ob.assigned_role
 LEFT JOIN project_role pmrr ON pmrr.code=pmr.role
 LEFT JOIN ` + "`user`" + ` sender ON sender.id=ob.sender_id
 LEFT JOIN campus_cooperation_application ca ON ca.id=sn.cooperation_application_id
 LEFT JOIN campus_cooperation c ON c.id=ca.cooperation_id
 LEFT JOIN commercial_order co ON co.id=c.commercial_order_id LEFT JOIN school ON school.id=c.school_id
 WHERE sn.user_id=? AND sn.displayed_at IS NULL AND (p.id IS NOT NULL OR ca.id IS NOT NULL)
 ORDER BY sn.priority DESC,sn.created_at DESC,sn.id DESC LIMIT 1`
	var n models.StatusNotification
	if err := r.db.GetContext(ctx, &n, query, userID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if _, err := r.db.ExecContext(ctx, `UPDATE status_notification SET displayed_at=CURRENT_TIMESTAMP WHERE user_id=? AND displayed_at IS NULL AND id<>?`, userID, n.ID); err != nil {
		return nil, err
	}
	return &n, nil
}
