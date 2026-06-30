package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

// OliveBranchRepository handles olive branch database operations
type OliveBranchRepository struct {
	db *sqlx.DB
}

// NewOliveBranchRepository creates a new OliveBranchRepository
func NewOliveBranchRepository(db *sqlx.DB) *OliveBranchRepository {
	return &OliveBranchRepository{db: db}
}

// OliveBranchListParams contains parameters for listing olive branches
type OliveBranchListParams struct {
	SenderID   int
	ReceiverID int
	Page       int
	Size       int
	Status     *int
}

// OliveBranchDashboardStats contains compact olive-branch metrics for dashboards.
type OliveBranchDashboardStats struct {
	Total    int
	Read     int
	Accepted int
	Handled  int
}

// obUserRow holds JOIN-ed user + school + major columns for olive branch queries.
type obUserRow struct {
	UID                 int      `db:"u_id"`
	UNickname           *string  `db:"u_nickname"`
	UPhone              *string  `db:"u_phone"`
	UEmail              *string  `db:"u_email"`
	UGrade              *int     `db:"u_grade"`
	UAuthStatus         *int     `db:"u_auth_status"`
	UAvatarUrl          *string  `db:"u_avatar_url"`
	UCollaborationScore *float64 `db:"u_collaboration_score"`
	USchoolID           *int     `db:"u_school_id"`
	UMajorID            *int     `db:"u_major_id"`
	USchoolName         *string  `db:"u_school_name"`
	USchoolCode         *string  `db:"u_school_code"`
	UMajorName          *string  `db:"u_major_name"`
	UClassID            *int     `db:"u_class_id"`
}

// obRow is the flat scan target (olive branch + user columns).
type obRow struct {
	models.OliveBranch
	obUserRow
	SmsID           *int       `db:"sms_id"`
	SmsOrderID      *int       `db:"sms_order_id"`
	SmsStatus       *int       `db:"sms_status"`
	SmsError        *string    `db:"sms_error_message"`
	SmsCreatedAt    *time.Time `db:"sms_created_at"`
	SmsUpdatedAt    *time.Time `db:"sms_updated_at"`
	CurrentUserRole *string    `db:"current_user_role"`
}

// ListByReceiverID retrieves paginated olive branches received by a user
func (r *OliveBranchRepository) ListByReceiverID(ctx context.Context, params OliveBranchListParams) ([]models.OliveBranch, int64, error) {
	// Count total
	countArgs := []interface{}{params.ReceiverID}
	countQuery := `SELECT COUNT(*) FROM olive_branch_record WHERE receiver_id = ?`
	if params.Status != nil {
		countQuery += ` AND status = ?`
		countArgs = append(countArgs, *params.Status)
	}

	var total int64
	if err := r.db.QueryRowxContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count olive branches: %w", err)
	}

	// Query with pagination
	offset := (params.Page - 1) * params.Size
	args := []interface{}{params.ReceiverID}

	query := `
		SELECT
			ob.id, ob.sender_id, ob.receiver_id, ob.related_project_id,
			ob.type, ob.cost_type, ob.status, ob.is_read,
			ob.created_at, ob.updated_at,
			p.name AS project_name,
			COALESCE(ob.operator_role, sender_member.role, p.publisher_role, 'TEAM_LEADER') AS operator_role,
			op_role.name AS operator_role_name,
			ob_member.role AS assigned_role,
			ob_role.name AS assigned_role_name,
			CASE WHEN ob_member.id IS NULL THEN FALSE ELSE TRUE END AS is_current_member,
			s.id          AS u_id,
			s.nickname    AS u_nickname,
			s.phone       AS u_phone,
			s.email       AS u_email,
			s.grade       AS u_grade,
			s.auth_status AS u_auth_status,
			s.avatar_url  AS u_avatar_url,
			s.collaboration_score AS u_collaboration_score,
			s.school_id   AS u_school_id,
			s.major_id    AS u_major_id,
			sch.school_name AS u_school_name,
			sch.school_code AS u_school_code,
			m.major_name  AS u_major_name,
			m.class_id    AS u_class_id
		FROM olive_branch_record ob
		LEFT JOIN project p ON ob.related_project_id = p.id
		LEFT JOIN project_members sender_member ON sender_member.project_id = ob.related_project_id AND sender_member.user_id = ob.sender_id
		LEFT JOIN project_role op_role ON op_role.code = COALESCE(ob.operator_role, sender_member.role, p.publisher_role, 'TEAM_LEADER')
		LEFT JOIN project_members ob_member ON ob_member.project_id = ob.related_project_id AND ob_member.user_id = ob.receiver_id
		LEFT JOIN project_role ob_role ON ob_role.code = ob_member.role
		LEFT JOIN ` + "`user`" + ` s ON ob.sender_id = s.id
		LEFT JOIN school sch ON s.school_id = sch.id
		LEFT JOIN major m ON s.major_id = m.id
		WHERE ob.receiver_id = ?
	`
	if params.Status != nil {
		query += ` AND ob.status = ?`
		args = append(args, *params.Status)
	}
	query += ` ORDER BY ob.created_at DESC LIMIT ? OFFSET ?`
	args = append(args, params.Size, offset)

	rows, err := r.db.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query olive branches: %w", err)
	}
	defer rows.Close()

	var records []models.OliveBranch
	for rows.Next() {
		var row obRow
		if err := rows.StructScan(&row); err != nil {
			return nil, 0, fmt.Errorf("scan olive branch: %w", err)
		}
		ob := row.OliveBranch
		ob.Sender = &models.User{
			ID:                 row.UID,
			Nickname:           models.DisplayNickname(row.UNickname),
			Phone:              row.UPhone,
			Email:              row.UEmail,
			Grade:              row.UGrade,
			AuthStatus:         row.UAuthStatus,
			AvatarUrl:          row.UAvatarUrl,
			CollaborationScore: row.UCollaborationScore,
			SchoolID:           row.USchoolID,
			MajorID:            row.UMajorID,
			SchoolName:         row.USchoolName,
			SchoolCode:         row.USchoolCode,
			MajorName:          row.UMajorName,
			ClassID:            row.UClassID,
		}
		records = append(records, ob)
	}
	rows.Close()

	if err := r.enrichSkills(ctx, records, func(ob *models.OliveBranch) *models.User { return ob.Sender }); err != nil {
		return nil, 0, err
	}

	return records, total, nil
}

// GetByID retrieves an olive branch by ID
func (r *OliveBranchRepository) GetByID(ctx context.Context, id int) (*models.OliveBranch, error) {
	query := `
		SELECT 
			ob.id, ob.sender_id, ob.receiver_id, ob.related_project_id,
			ob.type, ob.cost_type, ob.status,
			ob.created_at, ob.updated_at,
			p.name AS project_name,
			COALESCE(ob.operator_role, sender_member.role, p.publisher_role, 'TEAM_LEADER') AS operator_role,
			op_role.name AS operator_role_name
		FROM olive_branch_record ob
		LEFT JOIN project p ON ob.related_project_id = p.id
		LEFT JOIN project_members ob_member ON ob_member.project_id = ob.related_project_id AND ob_member.user_id = ob.receiver_id
		LEFT JOIN project_role ob_role ON ob_role.code = ob_member.role
		LEFT JOIN project_members sender_member ON sender_member.project_id = ob.related_project_id AND sender_member.user_id = ob.sender_id
		LEFT JOIN project_role op_role ON op_role.code = COALESCE(ob.operator_role, sender_member.role, p.publisher_role, 'TEAM_LEADER')
		WHERE ob.id = ?
	`

	var ob models.OliveBranch
	if err := r.db.QueryRowxContext(ctx, query, id).StructScan(&ob); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query olive branch by id: %w", err)
	}

	return &ob, nil
}

// Create creates a new olive branch record
func (r *OliveBranchRepository) Create(ctx context.Context, ob *models.OliveBranch) error {
	query := `
		INSERT INTO olive_branch_record (
			sender_id, receiver_id, related_project_id,
			type, cost_type, status, operator_role
		) VALUES (
			:sender_id, :receiver_id, :related_project_id,
			:type, :cost_type, :status, :operator_role
		)
	`

	result, err := r.db.NamedExecContext(ctx, query, ob)
	if err != nil {
		return fmt.Errorf("create olive branch: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	ob.ID = int(id)

	return nil
}

// CreateTx creates a new olive branch record within a transaction.
func (r *OliveBranchRepository) CreateTx(ctx context.Context, tx *sqlx.Tx, ob *models.OliveBranch) error {
	query := `
		INSERT INTO olive_branch_record (
			sender_id, receiver_id, related_project_id,
			type, cost_type, status, operator_role
		) VALUES (
			:sender_id, :receiver_id, :related_project_id,
			:type, :cost_type, :status, :operator_role
		)
	`

	result, err := tx.NamedExecContext(ctx, query, ob)
	if err != nil {
		return fmt.Errorf("create olive branch: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	ob.ID = int(id)

	return nil
}

// ExistsPending checks if there is a pending (status=0) olive branch from sender to receiver.
func (r *OliveBranchRepository) ExistsPending(ctx context.Context, senderID, receiverID, relatedProjectID int) (bool, error) {
	var count int
	query := `SELECT COUNT(*) FROM olive_branch_record WHERE sender_id = ? AND receiver_id = ? AND related_project_id = ? AND status IN (?, ?)`
	if err := r.db.QueryRowxContext(ctx, query, senderID, receiverID, relatedProjectID, models.OliveBranchStatusPending, models.OliveBranchStatusDiscussing).Scan(&count); err != nil {
		return false, fmt.Errorf("check pending olive branch: %w", err)
	}
	return count > 0, nil
}

// UpdateStatus updates the status of an olive branch
func (r *OliveBranchRepository) UpdateStatus(ctx context.Context, id int, status int) error {
	query := `UPDATE olive_branch_record SET status = ?,
		discussing_at=CASE WHEN ?=? THEN CURRENT_TIMESTAMP ELSE discussing_at END,
		rejected_at=CASE WHEN ?=? THEN CURRENT_TIMESTAMP ELSE rejected_at END,
		updated_at = CURRENT_TIMESTAMP WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query, status, status, models.OliveBranchStatusDiscussing, status, models.OliveBranchStatusRejected, id)
	if err != nil {
		return fmt.Errorf("update olive branch status: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("olive branch not found")
	}

	return nil
}

// ListBySenderID retrieves paginated olive branches sent by a user
func (r *OliveBranchRepository) ListBySenderID(ctx context.Context, params OliveBranchListParams) ([]models.OliveBranch, int64, error) {
	// Count total
	countArgs := []interface{}{params.SenderID, params.SenderID, params.SenderID}
	countQuery := `SELECT COUNT(*)
		FROM olive_branch_record ob
		LEFT JOIN project p ON ob.related_project_id = p.id
		LEFT JOIN project_members ob_member ON ob_member.project_id = ob.related_project_id AND ob_member.user_id = ob.receiver_id
		LEFT JOIN project_role ob_role ON ob_role.code = ob_member.role
		LEFT JOIN project_members sender_member ON sender_member.project_id = ob.related_project_id AND sender_member.user_id = ob.sender_id
		LEFT JOIN project_role op_role ON op_role.code = COALESCE(ob.operator_role, sender_member.role, p.publisher_role, 'TEAM_LEADER')
		WHERE (
			ob.sender_id = ?
			OR p.creator_id = ?
			OR EXISTS (
				SELECT 1 FROM project_members pm
				WHERE pm.project_id = ob.related_project_id AND pm.user_id = ?
			)
		)`
	if params.Status != nil {
		countQuery += ` AND ob.status = ?`
		countArgs = append(countArgs, *params.Status)
	}

	var total int64
	if err := r.db.QueryRowxContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count olive branches: %w", err)
	}

	// Query with pagination
	offset := (params.Page - 1) * params.Size
	args := []interface{}{params.SenderID, params.SenderID, params.SenderID, params.SenderID, params.SenderID, params.SenderID}

	query := `
		SELECT
			ob.id, ob.sender_id, ob.receiver_id, ob.related_project_id,
			ob.type, ob.cost_type, ob.status, ob.is_read,
			ob.created_at, ob.updated_at,
			p.name AS project_name,
			COALESCE(ob.operator_role, sender_member.role, p.publisher_role, 'TEAM_LEADER') AS operator_role,
			op_role.name AS operator_role_name,
			ob_member.role AS assigned_role,
			ob_role.name AS assigned_role_name,
			CASE WHEN ob_member.id IS NULL THEN FALSE ELSE TRUE END AS is_current_member,
			COALESCE(current_member.role, CASE WHEN p.creator_id = ? THEN p.publisher_role END, CASE WHEN p.creator_id = ? THEN 'TEAM_LEADER' END) AS current_user_role,
			recv.id          AS u_id,
			recv.nickname    AS u_nickname,
			recv.phone       AS u_phone,
			recv.email       AS u_email,
			recv.grade       AS u_grade,
			recv.auth_status AS u_auth_status,
			recv.avatar_url  AS u_avatar_url,
			recv.collaboration_score AS u_collaboration_score,
			recv.school_id   AS u_school_id,
			recv.major_id    AS u_major_id,
			sch.school_name  AS u_school_name,
			sch.school_code  AS u_school_code,
			m.major_name     AS u_major_name,
			m.class_id       AS u_class_id,
			sn.id            AS sms_id,
			sn.order_id      AS sms_order_id,
			sn.status        AS sms_status,
			sn.error_message AS sms_error_message,
			sn.created_at    AS sms_created_at,
			sn.updated_at    AS sms_updated_at
		FROM olive_branch_record ob
		LEFT JOIN project p ON ob.related_project_id = p.id
		LEFT JOIN project_members ob_member ON ob_member.project_id = ob.related_project_id AND ob_member.user_id = ob.receiver_id
		LEFT JOIN project_role ob_role ON ob_role.code = ob_member.role
		LEFT JOIN project_members sender_member ON sender_member.project_id = ob.related_project_id AND sender_member.user_id = ob.sender_id
		LEFT JOIN project_role op_role ON op_role.code = COALESCE(ob.operator_role, sender_member.role, p.publisher_role, 'TEAM_LEADER')
		LEFT JOIN project_members current_member ON current_member.project_id = ob.related_project_id AND current_member.user_id = ?
		LEFT JOIN ` + "`user`" + ` recv ON ob.receiver_id = recv.id
		LEFT JOIN school sch ON recv.school_id = sch.id
		LEFT JOIN major m ON recv.major_id = m.id
		LEFT JOIN olive_branch_sms_notice sn ON sn.olive_branch_record_id = ob.id
			AND sn.business_tag = 'olive_branch_sms_notice'
			AND sn.updated_at >= ob.updated_at
		WHERE (
			ob.sender_id = ?
			OR p.creator_id = ?
			OR EXISTS (
				SELECT 1 FROM project_members pm
				WHERE pm.project_id = ob.related_project_id AND pm.user_id = ?
			)
		)
	`
	if params.Status != nil {
		query += ` AND ob.status = ?`
		args = append(args, *params.Status)
	}
	query += ` ORDER BY ob.created_at DESC LIMIT ? OFFSET ?`
	args = append(args, params.Size, offset)

	rows, err := r.db.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query olive branches: %w", err)
	}
	defer rows.Close()

	var records []models.OliveBranch
	for rows.Next() {
		var row obRow
		if err := rows.StructScan(&row); err != nil {
			return nil, 0, fmt.Errorf("scan olive branch: %w", err)
		}
		ob := row.OliveBranch
		ob.Receiver = &models.User{
			ID:                 row.UID,
			Nickname:           models.DisplayNickname(row.UNickname),
			Phone:              row.UPhone,
			Email:              row.UEmail,
			Grade:              row.UGrade,
			AuthStatus:         row.UAuthStatus,
			AvatarUrl:          row.UAvatarUrl,
			CollaborationScore: row.UCollaborationScore,
			SchoolID:           row.USchoolID,
			MajorID:            row.UMajorID,
			SchoolName:         row.USchoolName,
			SchoolCode:         row.USchoolCode,
			MajorName:          row.UMajorName,
			ClassID:            row.UClassID,
		}
		if row.CurrentUserRole != nil && ob.OperatorRole != nil {
			canReview := canOperateOliveByRole(*row.CurrentUserRole, *ob.OperatorRole)
			ob.CanReview = &canReview
		}
		if row.SmsID != nil {
			ob.SmsNotice = &models.SmsNotice{
				ID:                  *row.SmsID,
				OrderID:             intValue(row.SmsOrderID),
				Status:              models.SmsNoticeStatus(intValue(row.SmsStatus)),
				ErrorMessage:        row.SmsError,
				CreatedAt:           timeValue(row.SmsCreatedAt),
				UpdatedAt:           timeValue(row.SmsUpdatedAt),
				OliveBranchRecordID: ob.ID,
			}
		}
		records = append(records, ob)
	}
	rows.Close()

	if err := r.enrichSkills(ctx, records, func(ob *models.OliveBranch) *models.User { return ob.Receiver }); err != nil {
		return nil, 0, err
	}

	return records, total, nil
}

func (r *OliveBranchRepository) GetProjectDashboardStats(ctx context.Context, projectID int) (OliveBranchDashboardStats, error) {
	var stats OliveBranchDashboardStats
	err := r.db.QueryRowxContext(ctx, `
		SELECT
			COUNT(*) AS total,
			COALESCE(SUM(CASE WHEN is_read = TRUE THEN 1 ELSE 0 END), 0) AS read_count,
			COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0) AS accepted,
			COALESCE(SUM(CASE WHEN status IN (?, ?) THEN 1 ELSE 0 END), 0) AS handled
		FROM olive_branch_record
		WHERE related_project_id = ?
	`, models.OliveBranchStatusAccepted, models.OliveBranchStatusAccepted, models.OliveBranchStatusRejected, projectID).Scan(
		&stats.Total,
		&stats.Read,
		&stats.Accepted,
		&stats.Handled,
	)
	if err != nil {
		return stats, fmt.Errorf("get project olive branch dashboard stats: %w", err)
	}
	return stats, nil
}

func (r *OliveBranchRepository) GetUserReceivedDashboardStats(ctx context.Context, userID int) (OliveBranchDashboardStats, error) {
	var stats OliveBranchDashboardStats
	err := r.db.QueryRowxContext(ctx, `
		SELECT
			COUNT(*) AS total,
			COALESCE(SUM(CASE WHEN is_read = TRUE THEN 1 ELSE 0 END), 0) AS read_count,
			COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0) AS accepted,
			COALESCE(SUM(CASE WHEN status IN (?, ?) THEN 1 ELSE 0 END), 0) AS handled
		FROM olive_branch_record
		WHERE receiver_id = ?
	`, models.OliveBranchStatusAccepted, models.OliveBranchStatusAccepted, models.OliveBranchStatusRejected, userID).Scan(
		&stats.Total,
		&stats.Read,
		&stats.Accepted,
		&stats.Handled,
	)
	if err != nil {
		return stats, fmt.Errorf("get user received olive branch dashboard stats: %w", err)
	}
	return stats, nil
}

func oliveRolePriority(role string) int {
	switch role {
	case models.ProjectRoleTeamLeader:
		return 1
	case models.ProjectRoleTeamMember, "":
		return 3
	default:
		return 2
	}
}

func canOperateOliveByRole(currentRole, operatorRole string) bool {
	currentPriority := oliveRolePriority(currentRole)
	operatorPriority := oliveRolePriority(operatorRole)
	if currentPriority == 1 {
		return true
	}
	return currentPriority <= operatorPriority
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func timeValue(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

// OliveBranchByProjectParams contains parameters for listing olive branches by project
type OliveBranchByProjectParams struct {
	ProjectID int
	Page      int
	Size      int
}

// ListByRelatedProjectID retrieves paginated olive branches sent for a project (receiver-focused)
func (r *OliveBranchRepository) ListByRelatedProjectID(ctx context.Context, params OliveBranchByProjectParams) ([]models.OliveBranch, int64, error) {
	// Count total
	var total int64
	if err := r.db.QueryRowxContext(ctx,
		`SELECT COUNT(*) FROM olive_branch_record WHERE related_project_id = ?`,
		params.ProjectID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count olive branches by project: %w", err)
	}

	offset := (params.Page - 1) * params.Size
	query := `
		SELECT
			ob.id, ob.sender_id, ob.receiver_id, ob.related_project_id,
			ob.type, ob.cost_type, ob.status, ob.is_read,
			ob.created_at, ob.updated_at,
			p.name AS project_name,
			COALESCE(ob.operator_role, sender_member.role, p.publisher_role, 'TEAM_LEADER') AS operator_role,
			op_role.name AS operator_role_name,
			ob_member.role AS assigned_role,
			ob_role.name AS assigned_role_name,
			CASE WHEN ob_member.id IS NULL THEN FALSE ELSE TRUE END AS is_current_member,
			recv.id          AS u_id,
			recv.nickname    AS u_nickname,
			recv.phone       AS u_phone,
			recv.email       AS u_email,
			recv.grade       AS u_grade,
			recv.auth_status AS u_auth_status,
			recv.avatar_url  AS u_avatar_url,
			recv.collaboration_score AS u_collaboration_score,
			recv.school_id   AS u_school_id,
			recv.major_id    AS u_major_id,
			sch.school_name  AS u_school_name,
			sch.school_code  AS u_school_code,
			m.major_name     AS u_major_name,
			m.class_id       AS u_class_id
		FROM olive_branch_record ob
		LEFT JOIN project p ON ob.related_project_id = p.id
		LEFT JOIN project_members ob_member ON ob_member.project_id = ob.related_project_id AND ob_member.user_id = ob.receiver_id
		LEFT JOIN project_role ob_role ON ob_role.code = ob_member.role
		LEFT JOIN project_members sender_member ON sender_member.project_id = ob.related_project_id AND sender_member.user_id = ob.sender_id
		LEFT JOIN project_role op_role ON op_role.code = COALESCE(ob.operator_role, sender_member.role, p.publisher_role, 'TEAM_LEADER')
		LEFT JOIN ` + "`user`" + ` recv ON ob.receiver_id = recv.id
		LEFT JOIN school sch ON recv.school_id = sch.id
		LEFT JOIN major m ON recv.major_id = m.id
		WHERE ob.related_project_id = ?
		ORDER BY ob.created_at DESC LIMIT ? OFFSET ?
	`

	rows, err := r.db.QueryxContext(ctx, query, params.ProjectID, params.Size, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query olive branches by project: %w", err)
	}
	defer rows.Close()

	var records []models.OliveBranch
	for rows.Next() {
		var row obRow
		if err := rows.StructScan(&row); err != nil {
			return nil, 0, fmt.Errorf("scan olive branch: %w", err)
		}
		ob := row.OliveBranch
		ob.Receiver = &models.User{
			ID:                 row.UID,
			Nickname:           models.DisplayNickname(row.UNickname),
			Phone:              row.UPhone,
			Email:              row.UEmail,
			Grade:              row.UGrade,
			AuthStatus:         row.UAuthStatus,
			AvatarUrl:          row.UAvatarUrl,
			CollaborationScore: row.UCollaborationScore,
			SchoolID:           row.USchoolID,
			MajorID:            row.UMajorID,
			SchoolName:         row.USchoolName,
			SchoolCode:         row.USchoolCode,
			MajorName:          row.UMajorName,
			ClassID:            row.UClassID,
		}
		records = append(records, ob)
	}
	rows.Close()

	return records, total, nil
}

// OliveBranchBadgeCounts holds badge counts for the olive branch feature.
type OliveBranchBadgeCounts struct {
	ReceivedPendingCount int
	SentUnreadCount      int
}

// GetBadgeCounts returns:
//   - ReceivedPendingCount: number of olive branches received by userID with status=0 (pending)
//   - SentUnreadCount:      number of olive branches sent by userID whose updated_at is after the
//     user's sent_olive_viewed_at (all sent branches if sent_olive_viewed_at is NULL).
//     Using updated_at ensures the count increments both when A first sends (updated_at = created_at)
//     and when B accepts/rejects (UpdateStatus sets updated_at = CURRENT_TIMESTAMP).
func (r *OliveBranchRepository) GetBadgeCounts(ctx context.Context, userID int) (OliveBranchBadgeCounts, error) {
	var counts OliveBranchBadgeCounts

	// 1. received pending count
	if err := r.db.QueryRowxContext(ctx,
		`SELECT COUNT(*) FROM olive_branch_record WHERE receiver_id = ? AND status = 0`,
		userID,
	).Scan(&counts.ReceivedPendingCount); err != nil {
		return counts, fmt.Errorf("count received pending: %w", err)
	}

	// 2. fetch sent_olive_viewed_at for this user
	var viewedAt *time.Time
	if err := r.db.QueryRowxContext(ctx,
		"SELECT sent_olive_viewed_at FROM `user` WHERE id = ?",
		userID,
	).Scan(&viewedAt); err != nil && err != sql.ErrNoRows {
		return counts, fmt.Errorf("get sent_olive_viewed_at: %w", err)
	}

	// 3. sent unread count — use updated_at so that a receiver's accept/reject
	//    (which sets updated_at = CURRENT_TIMESTAMP) causes the badge to reappear
	//    for the sender even after they previously called mark-sent-read.
	var sentQuery string
	var sentArgs []interface{}
	if viewedAt == nil {
		sentQuery = `SELECT COUNT(*) FROM olive_branch_record WHERE sender_id = ?`
		sentArgs = []interface{}{userID}
	} else {
		sentQuery = `SELECT COUNT(*) FROM olive_branch_record WHERE sender_id = ? AND updated_at > ?`
		sentArgs = []interface{}{userID, *viewedAt}
	}
	if err := r.db.QueryRowxContext(ctx, sentQuery, sentArgs...).Scan(&counts.SentUnreadCount); err != nil {
		return counts, fmt.Errorf("count sent unread: %w", err)
	}

	return counts, nil
}

// MarkReceiverRead sets is_read = TRUE for olive branches received by receiverID.
// If ids is non-empty, only those specific records are updated; otherwise all unread records for the receiver are updated.
func (r *OliveBranchRepository) MarkReceiverRead(ctx context.Context, receiverID int, ids []int) error {
	var rowsAffected int64
	if len(ids) > 0 {
		query, args, err := sqlx.In(
			`UPDATE olive_branch_record SET is_read = TRUE WHERE receiver_id = ? AND id IN (?) AND is_read = FALSE`,
			receiverID, ids,
		)
		if err != nil {
			return fmt.Errorf("build mark receiver read IN query: %w", err)
		}
		query = r.db.Rebind(query)
		result, err := r.db.ExecContext(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("mark receiver read: %w", err)
		}
		rowsAffected, _ = result.RowsAffected()
	} else {
		result, err := r.db.ExecContext(ctx,
			`UPDATE olive_branch_record SET is_read = TRUE WHERE receiver_id = ? AND is_read = FALSE`,
			receiverID,
		)
		if err != nil {
			return fmt.Errorf("mark receiver read: %w", err)
		}
		rowsAffected, _ = result.RowsAffected()
	}
	log.Printf("mark receiver olive branch read updated: receiverID=%d ids=%v rowsAffected=%d", receiverID, ids, rowsAffected)
	return nil
}

// enrichSkills batch-queries talent_profile for the target users and sets User.Skills.
// getUser extracts the relevant user (sender or receiver) from each record.
func (r *OliveBranchRepository) enrichSkills(ctx context.Context, records []models.OliveBranch, getUser func(*models.OliveBranch) *models.User) error {
	userIDs := make([]int, 0, len(records))
	for i := range records {
		if u := getUser(&records[i]); u != nil {
			userIDs = append(userIDs, u.ID)
		}
	}
	if len(userIDs) == 0 {
		return nil
	}

	q, args, err := sqlx.In(`SELECT user_id, skill_summary FROM talent_profile WHERE user_id IN (?)`, userIDs)
	if err != nil {
		return fmt.Errorf("build skills IN query: %w", err)
	}
	q = r.db.Rebind(q)

	type skillRow struct {
		UserID       int                    `db:"user_id"`
		SkillSummary models.JSONStringArray `db:"skill_summary"`
	}
	var rows []skillRow
	if err := r.db.SelectContext(ctx, &rows, q, args...); err != nil {
		return fmt.Errorf("batch query skills: %w", err)
	}

	skillsMap := make(map[int][]string, len(rows))
	for _, row := range rows {
		if row.SkillSummary.Valid {
			skillsMap[row.UserID] = row.SkillSummary.Items
		}
	}

	for i := range records {
		if u := getUser(&records[i]); u != nil {
			if skills, ok := skillsMap[u.ID]; ok {
				u.Skills = skills
			}
		}
	}
	return nil
}
