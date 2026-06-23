package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

// PurgeDeletedProjectsBefore permanently removes projects that have stayed in
// deleting status until cutoff. It also clears project-related tables that do
// not consistently have database-level ON DELETE CASCADE constraints.
func (r *Repository) PurgeDeletedProjectsBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	var ids []int
	if err := r.db.SelectContext(ctx, &ids, `
		SELECT id
		FROM project
		WHERE status = ? AND deleted_at IS NOT NULL AND deleted_at <= ?
		ORDER BY deleted_at ASC, id ASC
	`, models.ProjectStatusDeleting, cutoff); err != nil {
		return 0, fmt.Errorf("list expired deleted projects: %w", err)
	}
	if len(ids) == 0 {
		return 0, nil
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin purge deleted projects: %w", err)
	}
	defer tx.Rollback()

	projectTables := []string{
		"email_promotion_recipient",
		"project_view_log",
		"project_like",
		"project_favorite",
		"project_share",
		"project_tag_relation",
		"project_milestones",
		"project_members",
		"project_event",
		"project_recommendation",
		"collaboration_score",
		"sms_notice",
	}
	for _, table := range projectTables {
		if err := execDeleteInTx(ctx, tx, "DELETE FROM "+table+" WHERE project_id IN (?)", ids); err != nil {
			return 0, fmt.Errorf("delete %s for expired projects: %w", table, err)
		}
	}
	if err := execDeleteInTx(ctx, tx, "DELETE FROM interaction_dashboard_view_state WHERE target_type = 'projects' AND target_id IN (?)", ids); err != nil {
		return 0, fmt.Errorf("delete interaction_dashboard_view_state for expired projects: %w", err)
	}
	if err := execDeleteInTx(ctx, tx, "DELETE FROM olive_branch_record WHERE related_project_id IN (?)", ids); err != nil {
		return 0, fmt.Errorf("delete olive_branch_record for expired projects: %w", err)
	}

	result, err := execInTx(ctx, tx, `
		DELETE FROM project
		WHERE id IN (?) AND status = ? AND deleted_at IS NOT NULL AND deleted_at <= ?
	`, ids, models.ProjectStatusDeleting, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete expired projects: %w", err)
	}
	deleted, _ := result.RowsAffected()

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit purge deleted projects: %w", err)
	}
	return deleted, nil
}

func execDeleteInTx(ctx context.Context, tx *sqlx.Tx, query string, ids []int) error {
	_, err := execInTx(ctx, tx, query, ids)
	return err
}

func execInTx(ctx context.Context, tx *sqlx.Tx, query string, args ...interface{}) (sql.Result, error) {
	query, expandedArgs, err := sqlx.In(query, args...)
	if err != nil {
		return nil, err
	}
	return tx.ExecContext(ctx, tx.Rebind(query), expandedArgs...)
}
