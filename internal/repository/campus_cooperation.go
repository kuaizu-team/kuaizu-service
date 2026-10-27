package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

type CampusCooperationRepository struct{ db *sqlx.DB }

func NewCampusCooperationRepository(db *sqlx.DB) *CampusCooperationRepository {
	return &CampusCooperationRepository{db: db}
}

// Both reads and writes resolve current authorization from the database.
// A positive financial relation is deliberately NOT sufficient for this business.
type CooperationAdminScope struct {
	Role      int
	SchoolIDs []int
}

func (s CooperationAdminScope) CanAccess(schoolID int, edit bool) bool {
	if s.Role == models.AdminRoleSuperAdmin {
		return true
	}
	if s.Role != models.AdminRoleSchoolSuperAdmin && (edit || s.Role != models.AdminRoleSchoolAdmin) {
		return false
	}
	for _, id := range s.SchoolIDs {
		if id == schoolID {
			return true
		}
	}
	return false
}

func (r *CampusCooperationRepository) AdminScope(ctx context.Context, q sqlx.QueryerContext, adminID int, lock bool) (CooperationAdminScope, error) {
	var admin struct {
		Role     int  `db:"role"`
		SchoolID *int `db:"school_id"`
		Status   int  `db:"status"`
	}
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	if err := sqlx.GetContext(ctx, q, &admin, `SELECT role,school_id,status FROM admin_user WHERE id=?`+suffix, adminID); err != nil {
		return CooperationAdminScope{}, err
	}
	scope := CooperationAdminScope{Role: admin.Role, SchoolIDs: []int{}}
	if admin.Status != models.AdminUserStatusEnabled {
		return CooperationAdminScope{}, sql.ErrNoRows
	}
	switch admin.Role {
	case models.AdminRoleSuperAdmin:
	case models.AdminRoleSchoolSuperAdmin:
		if err := sqlx.SelectContext(ctx, q, &scope.SchoolIDs, `SELECT school_id FROM admin_school_relation WHERE admin_user_id=? AND is_owner=1 ORDER BY school_id`+suffix, adminID); err != nil {
			return scope, err
		}
	case models.AdminRoleSchoolAdmin:
		if admin.SchoolID != nil {
			scope.SchoolIDs = append(scope.SchoolIDs, *admin.SchoolID)
		}
	default:
		return CooperationAdminScope{}, sql.ErrNoRows
	}
	return scope, nil
}

func HasCooperationTeam(ctx context.Context, q sqlx.QueryerContext, schoolID int) (bool, error) {
	var exists bool
	err := sqlx.GetContext(ctx, q, &exists, `SELECT EXISTS(SELECT 1 FROM admin_school_relation rel JOIN admin_user au ON au.id=rel.admin_user_id WHERE rel.school_id=? AND rel.is_owner=1 AND au.role=2 AND au.status=1)`, schoolID)
	return exists, err
}

const cooperationColumns = `c.id,c.commercial_order_id,c.school_id,c.claimed_by,c.claimed_at,c.title,c.introduction,c.tags,(SELECT COUNT(*) FROM campus_cooperation_view cv WHERE cv.cooperation_id=c.id) AS view_count,c.links_visible,c.links,c.images_visible,c.image_title,c.images,c.contacts_visible,c.contacts,c.application_required,c.published,c.updated_by,c.updated_at,o.name AS order_name,o.status AS order_status,o.content_template AS order_template,o.feishu_wiki_node_token,o.company_name,s.school_name`
const cooperationJoins = ` FROM campus_cooperation c JOIN commercial_order o ON o.id=c.commercial_order_id JOIN school s ON s.id=c.school_id`
const cooperationApplicationColumns = `a.id,a.cooperation_id,a.user_id,a.attempt_no,a.status,a.profile_snapshot,a.applied_at,a.reviewed_by,a.reviewed_at,a.review_remark,c.title AS cooperation_title,o.company_name,s.school_name`
const cooperationApplicationJoins = ` FROM campus_cooperation_application a JOIN campus_cooperation c ON c.id=a.cooperation_id JOIN commercial_order o ON o.id=c.commercial_order_id JOIN school s ON s.id=c.school_id`

func GetCampusCooperation(ctx context.Context, q sqlx.QueryerContext, id int64, lock bool) (*models.CampusCooperation, error) {
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	var c models.CampusCooperation
	if err := sqlx.GetContext(ctx, q, &c, `SELECT `+cooperationColumns+cooperationJoins+` WHERE c.id=?`+suffix, id); err != nil {
		return nil, err
	}
	if err := c.DecodeContent(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *CampusCooperationRepository) ListOrders(ctx context.Context, page, size int, status string, schoolIDs []int) ([]models.CommercialOrder, int64, error) {
	where, args := "1=1", []interface{}{}
	if status != "" {
		where = "status=?"
		args = append(args, status)
	}
	var total int64
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM commercial_order WHERE `+where, args...); err != nil {
		return nil, 0, err
	}
	rows := []models.CommercialOrder{}
	args = append(args, size, (page-1)*size)
	err := r.db.SelectContext(ctx, &rows, `SELECT id,name,company_name,description,feishu_wiki_node_token,status,content_template,created_by,updated_by,created_at,updated_at FROM commercial_order WHERE `+where+` ORDER BY created_at DESC,id DESC LIMIT ? OFFSET ?`, args...)
	if err == nil {
		for i := range rows {
			if err := rows[i].DecodeTemplate(); err != nil {
				return nil, 0, err
			}
		}
	}
	if err != nil || len(rows) == 0 || len(schoolIDs) == 0 {
		return rows, total, err
	}
	orderIDs := make([]int64, len(rows))
	byID := make(map[int64]*models.CommercialOrder, len(rows))
	for i := range rows {
		orderIDs[i] = rows[i].ID
		byID[rows[i].ID] = &rows[i]
		rows[i].ClaimedSchoolIDs = []int{}
	}
	query, claimArgs, err := sqlx.In(`SELECT commercial_order_id,school_id FROM campus_cooperation WHERE commercial_order_id IN (?) AND school_id IN (?)`, orderIDs, schoolIDs)
	if err != nil {
		return nil, 0, err
	}
	var claims []struct {
		OrderID  int64 `db:"commercial_order_id"`
		SchoolID int   `db:"school_id"`
	}
	if err := r.db.SelectContext(ctx, &claims, query, claimArgs...); err != nil {
		return nil, 0, err
	}
	for _, claim := range claims {
		byID[claim.OrderID].ClaimedSchoolIDs = append(byID[claim.OrderID].ClaimedSchoolIDs, claim.SchoolID)
	}
	return rows, total, err
}

func (r *CampusCooperationRepository) List(ctx context.Context, schoolIDs []int, publishedOnly bool, page, size int) ([]models.CampusCooperation, int64, error) {
	where, args := "1=1", []interface{}{}
	if schoolIDs != nil {
		if len(schoolIDs) == 0 {
			return []models.CampusCooperation{}, 0, nil
		}
		where = "c.school_id IN (?)"
		args = append(args, schoolIDs)
	}
	if publishedOnly {
		where += " AND c.published=1 AND o.status<>'ended'"
	}
	query, args, err := sqlx.In(`SELECT COUNT(*)`+cooperationJoins+` WHERE `+where, args...)
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := r.db.GetContext(ctx, &total, query, args...); err != nil {
		return nil, 0, err
	}
	// Rebuild the IN query before adding LIMIT/OFFSET; sqlx.In has already flattened args.
	listQuery := `SELECT ` + cooperationColumns + cooperationJoins + ` WHERE ` + where
	listArgs := []interface{}{}
	if schoolIDs != nil {
		listArgs = append(listArgs, schoolIDs)
	}
	listQuery, listArgs, err = sqlx.In(listQuery, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	listArgs = append(listArgs, size, (page-1)*size)
	rows := []models.CampusCooperation{}
	if err := r.db.SelectContext(ctx, &rows, listQuery+` ORDER BY c.claimed_at DESC,c.id DESC LIMIT ? OFFSET ?`, listArgs...); err != nil {
		return nil, 0, err
	}
	for i := range rows {
		if err := rows[i].DecodeContent(); err != nil {
			return nil, 0, err
		}
	}
	return rows, total, nil
}

func (r *CampusCooperationRepository) ListApplications(ctx context.Context, cooperationID int64, userID, page, size int) ([]models.CooperationApplication, int64, error) {
	where, args := "a.cooperation_id=?", []interface{}{cooperationID}
	if userID > 0 {
		where, args = "a.user_id=?", []interface{}{userID}
	}
	var total int64
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*)`+cooperationApplicationJoins+` WHERE `+where, args...); err != nil {
		return nil, 0, err
	}
	rows := []models.CooperationApplication{}
	args = append(args, size, (page-1)*size)
	if err := r.db.SelectContext(ctx, &rows, `SELECT `+cooperationApplicationColumns+cooperationApplicationJoins+` WHERE `+where+` ORDER BY a.applied_at DESC,a.id DESC LIMIT ? OFFSET ?`, args...); err != nil {
		return nil, 0, err
	}
	for i := range rows {
		if userID > 0 {
			rows[i].ReviewRemark = nil
			rows[i].ReviewedBy = nil
		} else if rows[i].ProfileJSON != nil {
			if !json.Valid([]byte(*rows[i].ProfileJSON)) {
				return nil, 0, fmt.Errorf("invalid application card snapshot")
			}
			rows[i].ProfileSnapshot = json.RawMessage(*rows[i].ProfileJSON)
		}
	}
	return rows, total, nil
}

func LatestCooperationApplication(ctx context.Context, q sqlx.QueryerContext, cooperationID int64, userID int, lock bool) (*models.CooperationApplication, error) {
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	var a models.CooperationApplication
	err := sqlx.GetContext(ctx, q, &a, `SELECT id,cooperation_id,user_id,attempt_no,status,profile_snapshot,applied_at,reviewed_by,reviewed_at,review_remark FROM campus_cooperation_application WHERE cooperation_id=? AND user_id=? ORDER BY attempt_no DESC LIMIT 1`+suffix, cooperationID, userID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &a, err
}

// CooperationResultTx stores a private-contact-free snapshot and a queue entry atomically.
func CooperationResultTx(ctx context.Context, tx *sqlx.Tx, a *models.CooperationApplication, c *models.CampusCooperation) (int64, error) {
	kind, priority := models.StatusNotificationCooperationRejected, 10
	if a.Status == "approved" {
		kind, priority = models.StatusNotificationCooperationApproved, 200
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO status_notification(user_id,type,cooperation_application_id,priority) VALUES(?,?,?,?)`, a.UserID, kind, a.ID, priority)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	payload, err := json.Marshal(map[string]interface{}{
		"cooperationId": c.ID, "cooperationTitle": c.Title, "companyName": c.CompanyName, "schoolName": c.SchoolName,
		"cooperationApplicationId": a.ID, "applicationStatus": a.Status, "appliedAt": a.AppliedAt, "reviewedAt": a.ReviewedAt, "eventAt": a.ReviewedAt,
	})
	if err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO user_status_message(user_id,type,source_key,status_notification_id,payload,fullscreen_displayed) VALUES(?,?,?,?,?,0)`, a.UserID, kind, fmt.Sprintf("cooperation-review:%d", a.ID), id, string(payload))
	return id, err
}

func (r *CampusCooperationRepository) RecordView(ctx context.Context, cooperationID int64, userID int) (int64, error) {
	if _, err := r.db.ExecContext(ctx, "INSERT IGNORE INTO campus_cooperation_view(cooperation_id,user_id) VALUES(?,?)", cooperationID, userID); err != nil {
		return 0, err
	}
	var count int64
	err := r.db.GetContext(ctx, &count, "SELECT COUNT(*) FROM campus_cooperation_view WHERE cooperation_id=?", cooperationID)
	return count, err
}

// SearchTags shares the existing project tag library and its literal LIKE escaping.
func (r *CampusCooperationRepository) SearchTags(ctx context.Context, keyword string, limit int) ([]models.ProjectTag, error) {
	tags := []models.ProjectTag{}
	err := r.db.SelectContext(ctx, &tags, `SELECT id,name FROM project_tag
		WHERE status=1 AND name LIKE ? ESCAPE '!'
		ORDER BY CASE WHEN name LIKE ? ESCAPE '!' THEN 0 ELSE 1 END,sort_order,id LIMIT ?`,
		roleTagLikePattern(keyword, false), roleTagLikePattern(keyword, true), limit)
	return tags, err
}
