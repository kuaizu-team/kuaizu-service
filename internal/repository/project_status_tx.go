package repository

import (
	"context"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

func UpdateProjectStatusTx(ctx context.Context, tx *sqlx.Tx, id, current, status int, reason *string) (bool, error) {
	res, err := tx.ExecContext(ctx, `UPDATE project SET
 status=?, reject_reason=CASE WHEN ?=? THEN ? ELSE NULL END,
 deleted_at=CASE WHEN ?=? THEN deleted_at ELSE NULL END,
 recruit_completed_at=CASE WHEN ?=? THEN CURRENT_TIMESTAMP ELSE recruit_completed_at END,
 ended_at=CASE WHEN ?=? THEN CURRENT_TIMESTAMP ELSE ended_at END,
 passive_status_changed_at=CASE WHEN ?=? AND ? IN (?,?) THEN CURRENT_TIMESTAMP ELSE passive_status_changed_at END,
 updated_at=CURRENT_TIMESTAMP WHERE id=? AND status=?`,
		status, status, models.ProjectStatusRejected, reason, status, models.ProjectStatusDeleting,
		status, models.ProjectStatusRecruitCompleted, status, models.ProjectStatusEnded,
		current, models.ProjectStatusPending, status, models.ProjectStatusApproved, models.ProjectStatusRejected, id, current)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}
