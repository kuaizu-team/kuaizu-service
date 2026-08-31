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

	if err := deleteProjectRelations(ctx, tx, ids); err != nil {
		return 0, err
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

// PurgeDeletedProjectBefore permanently removes one eligible deleting project.
// Eligibility is checked again in the final DELETE to protect against stale UI state.
func (r *Repository) PurgeDeletedProjectBefore(ctx context.Context, id int, cutoff time.Time) (int64, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin permanent project deletion: %w", err)
	}
	defer tx.Rollback()

	var eligibleID int
	if err := tx.GetContext(ctx, &eligibleID, `
		SELECT id FROM project
		WHERE id = ? AND status = ? AND deleted_at IS NOT NULL AND deleted_at < ?
		FOR UPDATE
	`, id, models.ProjectStatusDeleting, cutoff); err != nil {
		return 0, err
	}

	ids := []int{eligibleID}
	if err := deleteProjectRelations(ctx, tx, ids); err != nil {
		return 0, err
	}
	result, err := execInTx(ctx, tx, `
		DELETE FROM project
		WHERE id IN (?) AND status = ? AND deleted_at IS NOT NULL AND deleted_at < ?
	`, ids, models.ProjectStatusDeleting, cutoff)
	if err != nil {
		return 0, fmt.Errorf("permanently delete project: %w", err)
	}
	deleted, _ := result.RowsAffected()
	if deleted == 0 {
		return 0, sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit permanent project deletion: %w", err)
	}
	return deleted, nil
}

var projectRelationTables = []string{
	"email_promotion_recipient", "project_view_log", "project_like",
	"project_favorite", "project_share", "project_tag_relation",
	"project_milestones", "project_image",
	"project_member_rating", "project_member_score",
	"project_members", "project_event",
	"project_recommendation", "collaboration_score", "olive_branch_sms_notice",
}

func deleteProjectRelations(ctx context.Context, tx *sqlx.Tx, ids []int) error {
	if err := markProjectMediaForCleanup(ctx, tx, ids); err != nil {
		return err
	}
	if _, err := execInTx(ctx, tx, `DELETE pme FROM project_milestone_evidence pme
		INNER JOIN project_milestones pm ON pm.id=pme.milestone_id
		WHERE pm.project_id IN (?)`, ids); err != nil {
		return fmt.Errorf("delete milestone evidence for project: %w", err)
	}
	for _, table := range projectRelationTables {
		if err := execDeleteInTx(ctx, tx, "DELETE FROM "+table+" WHERE project_id IN (?)", ids); err != nil {
			return fmt.Errorf("delete %s for project: %w", table, err)
		}
	}
	if err := execDeleteInTx(ctx, tx, "DELETE FROM interaction_dashboard_view_state WHERE target_type = 'projects' AND target_id IN (?)", ids); err != nil {
		return fmt.Errorf("delete interaction_dashboard_view_state for project: %w", err)
	}
	if err := execDeleteInTx(ctx, tx, "DELETE FROM olive_branch_record WHERE related_project_id IN (?)", ids); err != nil {
		return fmt.Errorf("delete olive_branch_record for project: %w", err)
	}
	return nil
}

func markProjectMediaForCleanup(ctx context.Context, tx *sqlx.Tx, ids []int) error {
	if _, err := execInTx(ctx, tx, `UPDATE media_upload mu
		INNER JOIN project_image pi ON pi.object_key=mu.object_key
		SET mu.attached_type=?,mu.attached_id=NULL,mu.attached_at=CURRENT_TIMESTAMP
		WHERE pi.project_id IN (?)`, mediaAttachmentCleanup, ids); err != nil {
		return fmt.Errorf("mark project images for cleanup: %w", err)
	}
	if _, err := execInTx(ctx, tx, `UPDATE media_upload mu
		INNER JOIN project_milestone_evidence pme ON pme.object_key=mu.object_key
		INNER JOIN project_milestones pm ON pm.id=pme.milestone_id
		SET mu.attached_type=?,mu.attached_id=NULL,mu.attached_at=CURRENT_TIMESTAMP
		WHERE pm.project_id IN (?)`, mediaAttachmentCleanup, ids); err != nil {
		return fmt.Errorf("mark milestone evidence for cleanup: %w", err)
	}
	return nil
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
