package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
)

const (
	MediaTypeProjectImage      = "project_image"
	MediaTypeTalentWork        = "talent_work"
	MediaTypeMilestoneEvidence = "milestone_evidence"
)

type MediaRepository struct{ db *sqlx.DB }

func NewMediaRepository(db *sqlx.DB) *MediaRepository { return &MediaRepository{db: db} }

func (r *MediaRepository) RegisterUpload(ctx context.Context, key string, ownerUserID int, mediaType string) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO media_upload(object_key,owner_user_id,media_type) VALUES(?,?,?)`, key, ownerUserID, mediaType)
	return err
}

func (r *MediaRepository) ListProjectImages(ctx context.Context, projectID int) ([]string, error) {
	var keys []string
	err := r.db.SelectContext(ctx, &keys, `SELECT object_key FROM project_image WHERE project_id=? ORDER BY sort_order,id`, projectID)
	return keys, err
}

func (r *MediaRepository) ListTalentWorkImages(ctx context.Context, profileID int) ([]string, error) {
	var keys []string
	err := r.db.SelectContext(ctx, &keys, `SELECT object_key FROM talent_work_image WHERE talent_profile_id=? ORDER BY sort_order,id`, profileID)
	return keys, err
}

func (r *MediaRepository) ReplaceProjectImages(ctx context.Context, ownerUserID, projectID int, keys []string) ([]string, error) {
	return r.replaceImages(ctx, ownerUserID, MediaTypeProjectImage, "project", projectID, "project_image", "project_id", "project-images/", 6, keys)
}

func (r *MediaRepository) ReplaceTalentWorkImages(ctx context.Context, ownerUserID, profileID int, keys []string) ([]string, error) {
	return r.replaceImages(ctx, ownerUserID, MediaTypeTalentWork, "talent_profile", profileID, "talent_work_image", "talent_profile_id", "talent-work-images/", 5, keys)
}

func (r *MediaRepository) replaceImages(ctx context.Context, ownerUserID int, mediaType, attachedType string, attachedID int, table, idColumn, prefix string, max int, keys []string) ([]string, error) {
	keys, err := normalizeOwnedKeys(keys, prefix, max)
	if err != nil {
		return nil, err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := validateOwnedUploads(ctx, tx, ownerUserID, mediaType, attachedType, attachedID, keys); err != nil {
		return nil, err
	}
	var old []string
	if err := tx.SelectContext(ctx, &old, `SELECT object_key FROM `+table+` WHERE `+idColumn+`=?`, attachedID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE `+idColumn+`=?`, attachedID); err != nil {
		return nil, err
	}
	for i, key := range keys {
		if _, err := tx.ExecContext(ctx, `INSERT INTO `+table+`(`+idColumn+`,object_key,sort_order) VALUES(?,?,?)`, attachedID, key, i+1); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE media_upload SET attached_type=?,attached_id=?,attached_at=CURRENT_TIMESTAMP WHERE object_key=?`, attachedType, attachedID, key); err != nil {
			return nil, err
		}
	}
	keep := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		keep[key] = struct{}{}
	}
	removed := make([]string, 0)
	for _, key := range old {
		if _, ok := keep[key]; ok {
			continue
		}
		removed = append(removed, key)
		if _, err := tx.ExecContext(ctx, `UPDATE media_upload SET attached_type=NULL,attached_id=NULL,attached_at=NULL WHERE object_key=? AND owner_user_id=?`, key, ownerUserID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return removed, nil
}

func normalizeOwnedKeys(keys []string, prefix string, max int) ([]string, error) {
	if len(keys) > max {
		return nil, fmt.Errorf("image count exceeds limit")
	}
	result := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, raw := range keys {
		key := strings.TrimSpace(raw)
		if key == "" || !strings.HasPrefix(key, prefix) || strings.Contains(key, "..") {
			return nil, fmt.Errorf("invalid image key")
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, key)
	}
	return result, nil
}

func validateOwnedUploads(ctx context.Context, tx *sqlx.Tx, ownerUserID int, mediaType, attachedType string, attachedID int, keys []string) error {
	for _, key := range keys {
		var count int
		err := tx.GetContext(ctx, &count, `SELECT COUNT(*) FROM media_upload WHERE object_key=? AND owner_user_id=? AND media_type=? AND (attached_type IS NULL OR (attached_type=? AND attached_id=?))`, key, ownerUserID, mediaType, attachedType, attachedID)
		if err != nil || count != 1 {
			return fmt.Errorf("image key is not owned by current user")
		}
	}
	return nil
}

func (r *MediaRepository) SubmitMilestoneEvidence(ctx context.Context, ownerUserID, projectID, milestoneID int, keys []string) ([]string, error) {
	keys, err := normalizeOwnedKeys(keys, "milestone-evidence/", 9)
	if err != nil || len(keys) == 0 {
		return nil, fmt.Errorf("at least one valid evidence image is required")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var count int
	if err := tx.GetContext(ctx, &count, `SELECT COUNT(*) FROM project_milestones pm JOIN project p ON p.id=pm.project_id WHERE pm.id=? AND pm.project_id=? AND p.creator_id=? AND pm.certification_status IN (0,3)`, milestoneID, projectID, ownerUserID); err != nil || count != 1 {
		return nil, fmt.Errorf("milestone is not owned by current user")
	}
	if err := validateOwnedUploads(ctx, tx, ownerUserID, MediaTypeMilestoneEvidence, "milestone", milestoneID, keys); err != nil {
		return nil, err
	}
	var old []string
	if err := tx.SelectContext(ctx, &old, `SELECT object_key FROM project_milestone_evidence WHERE milestone_id=?`, milestoneID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM project_milestone_evidence WHERE milestone_id=?`, milestoneID); err != nil {
		return nil, err
	}
	for i, key := range keys {
		if _, err := tx.ExecContext(ctx, `INSERT INTO project_milestone_evidence(milestone_id,object_key,sort_order) VALUES(?,?,?)`, milestoneID, key, i+1); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE media_upload SET attached_type='milestone',attached_id=?,attached_at=CURRENT_TIMESTAMP WHERE object_key=?`, milestoneID, key); err != nil {
			return nil, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE project_milestones SET certification_status=1,updated_at=CURRENT_TIMESTAMP WHERE id=?`, milestoneID); err != nil {
		return nil, err
	}
	keep := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		keep[key] = struct{}{}
	}
	removed := make([]string, 0)
	for _, key := range old {
		if _, ok := keep[key]; ok {
			continue
		}
		removed = append(removed, key)
		if _, err := tx.ExecContext(ctx, `UPDATE media_upload SET attached_type=NULL,attached_id=NULL,attached_at=NULL WHERE object_key=? AND owner_user_id=?`, key, ownerUserID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return removed, nil
}

func (r *MediaRepository) EvidenceKeys(ctx context.Context, milestoneID int) ([]string, error) {
	var keys []string
	err := r.db.SelectContext(ctx, &keys, `SELECT object_key FROM project_milestone_evidence WHERE milestone_id=? ORDER BY sort_order,id`, milestoneID)
	return keys, err
}

func (r *MediaRepository) FinalizeMilestoneReview(ctx context.Context, milestoneID, status int) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE project_milestones SET certification_status=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND certification_status=1`, status, milestoneID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return fmt.Errorf("milestone certification is not pending")
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM media_upload WHERE object_key IN (SELECT object_key FROM project_milestone_evidence WHERE milestone_id=?)`, milestoneID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM project_milestone_evidence WHERE milestone_id=?`, milestoneID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *MediaRepository) DeleteUploadRecords(ctx context.Context, ownerUserID int, keys []string) error {
	for _, key := range keys {
		if _, err := r.db.ExecContext(ctx, `DELETE FROM media_upload WHERE object_key=? AND owner_user_id=? AND attached_type IS NULL`, key, ownerUserID); err != nil {
			return err
		}
	}
	return nil
}
