package repository

import (
	"context"
	"fmt"
	"github.com/jmoiron/sqlx"
)

// CanViewContacts accepts only current teamwork or an active discussing relation.
// Completed/rejected history does not grant contacts after a member leaves.
func CanViewContacts(ctx context.Context, db *sqlx.DB, viewerID, targetID int) (bool, error) {
	if viewerID <= 0 || targetID <= 0 {
		return false, nil
	}
	if viewerID == targetID {
		return true, nil
	}
	if db == nil {
		return false, fmt.Errorf("contact access store is unavailable")
	}
	var allowed bool
	err := db.GetContext(ctx, &allowed, `
SELECT
 EXISTS(SELECT 1 FROM project p
   WHERE p.status<>4 AND p.deleted_at IS NULL
   AND (p.creator_id=? OR EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=p.id AND pm.user_id=?))
   AND (p.creator_id=? OR EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=p.id AND pm.user_id=?)))
 OR EXISTS(SELECT 1 FROM project_application pa JOIN project p ON p.id=pa.project_id
   WHERE pa.status=1 AND p.status<>4 AND p.deleted_at IS NULL AND (
     (pa.user_id=? AND (p.creator_id=? OR (pa.reviewer_id=? AND EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=p.id AND pm.user_id=?))))
     OR (pa.user_id=? AND (p.creator_id=? OR (pa.reviewer_id=? AND EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=p.id AND pm.user_id=?))))
   ))
 OR EXISTS(SELECT 1 FROM olive_branch_record ob JOIN project p ON p.id=ob.related_project_id
   WHERE ob.status=4 AND p.status<>4 AND p.deleted_at IS NULL
   AND ((ob.sender_id=? AND ob.receiver_id=?) OR (ob.sender_id=? AND ob.receiver_id=?))
   AND (p.creator_id=ob.sender_id OR EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=p.id AND pm.user_id=ob.sender_id)))
`, viewerID, viewerID, targetID, targetID,
		targetID, viewerID, viewerID, viewerID, viewerID, targetID, targetID, targetID,
		viewerID, targetID, targetID, viewerID)
	return allowed, err
}

// CanReadSubmittedProfile permits a project reviewer to read submitted profile
// content while the application is pending. It never grants contact visibility.
func CanReadSubmittedProfile(ctx context.Context, db *sqlx.DB, viewerID, targetID int) (bool, error) {
	var allowed bool
	err := db.GetContext(ctx, &allowed, `SELECT EXISTS(
 SELECT 1 FROM project_application pa JOIN project p ON p.id=pa.project_id
 WHERE pa.user_id=? AND pa.status=0 AND p.status<>4 AND p.deleted_at IS NULL
 AND (p.creator_id=? OR EXISTS(
 SELECT 1 FROM project_members pm WHERE pm.project_id=p.id AND pm.user_id=?)))`, targetID, viewerID, viewerID)
	return allowed, err
}
