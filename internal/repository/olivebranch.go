package repository

import (
	"context"
	"database/sql"
	"fmt"

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

// obUserRow holds JOIN-ed user + school + major columns for olive branch queries.
type obUserRow struct {
	UID         int     `db:"u_id"`
	UNickname   *string `db:"u_nickname"`
	UPhone      *string `db:"u_phone"`
	UEmail      *string `db:"u_email"`
	UGrade      *int    `db:"u_grade"`
	UAuthStatus *int    `db:"u_auth_status"`
	UAvatarUrl  *string `db:"u_avatar_url"`
	USchoolID   *int    `db:"u_school_id"`
	UMajorID    *int    `db:"u_major_id"`
	USchoolName *string `db:"u_school_name"`
	USchoolCode *string `db:"u_school_code"`
	UMajorName  *string `db:"u_major_name"`
	UClassID    *int    `db:"u_class_id"`
}

// obRow is the flat scan target (olive branch + user columns).
type obRow struct {
	models.OliveBranch
	obUserRow
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
			ob.type, ob.cost_type, ob.status,
			ob.created_at, ob.updated_at,
			p.name AS project_name,
			s.id          AS u_id,
			s.nickname    AS u_nickname,
			s.phone       AS u_phone,
			s.email       AS u_email,
			s.grade       AS u_grade,
			s.auth_status AS u_auth_status,
			s.avatar_url  AS u_avatar_url,
			s.school_id   AS u_school_id,
			s.major_id    AS u_major_id,
			sch.school_name AS u_school_name,
			sch.school_code AS u_school_code,
			m.major_name  AS u_major_name,
			m.class_id    AS u_class_id
		FROM olive_branch_record ob
		LEFT JOIN project p ON ob.related_project_id = p.id
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
			ID:         row.UID,
			Nickname:   row.UNickname,
			Phone:      row.UPhone,
			Email:      row.UEmail,
			Grade:      row.UGrade,
			AuthStatus: row.UAuthStatus,
			AvatarUrl:  row.UAvatarUrl,
			SchoolID:   row.USchoolID,
			MajorID:    row.UMajorID,
			SchoolName: row.USchoolName,
			SchoolCode: row.USchoolCode,
			MajorName:  row.UMajorName,
			ClassID:    row.UClassID,
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
			p.name AS project_name
		FROM olive_branch_record ob
		LEFT JOIN project p ON ob.related_project_id = p.id
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
			type, cost_type, status
		) VALUES (
			:sender_id, :receiver_id, :related_project_id,
			:type, :cost_type, :status
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

// ExistsPending checks if there is a pending (status=0) olive branch from sender to receiver.
func (r *OliveBranchRepository) ExistsPending(ctx context.Context, senderID, receiverID, relatedProjectID int) (bool, error) {
	var count int
	query := `SELECT COUNT(*) FROM olive_branch_record WHERE sender_id = ? AND receiver_id = ? AND related_project_id = ? AND status = 0`
	if err := r.db.QueryRowxContext(ctx, query, senderID, receiverID, relatedProjectID).Scan(&count); err != nil {
		return false, fmt.Errorf("check pending olive branch: %w", err)
	}
	return count > 0, nil
}

// UpdateStatus updates the status of an olive branch
func (r *OliveBranchRepository) UpdateStatus(ctx context.Context, id int, status int) error {
	query := `UPDATE olive_branch_record SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query, status, id)
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
	countArgs := []interface{}{params.SenderID}
	countQuery := `SELECT COUNT(*) FROM olive_branch_record WHERE sender_id = ?`
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
	args := []interface{}{params.SenderID}

	query := `
		SELECT
			ob.id, ob.sender_id, ob.receiver_id, ob.related_project_id,
			ob.type, ob.cost_type, ob.status,
			ob.created_at, ob.updated_at,
			p.name AS project_name,
			recv.id          AS u_id,
			recv.nickname    AS u_nickname,
			recv.phone       AS u_phone,
			recv.email       AS u_email,
			recv.grade       AS u_grade,
			recv.auth_status AS u_auth_status,
			recv.avatar_url  AS u_avatar_url,
			recv.school_id   AS u_school_id,
			recv.major_id    AS u_major_id,
			sch.school_name  AS u_school_name,
			sch.school_code  AS u_school_code,
			m.major_name     AS u_major_name,
			m.class_id       AS u_class_id
		FROM olive_branch_record ob
		LEFT JOIN project p ON ob.related_project_id = p.id
		LEFT JOIN ` + "`user`" + ` recv ON ob.receiver_id = recv.id
		LEFT JOIN school sch ON recv.school_id = sch.id
		LEFT JOIN major m ON recv.major_id = m.id
		WHERE ob.sender_id = ?
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
			ID:         row.UID,
			Nickname:   row.UNickname,
			Phone:      row.UPhone,
			Email:      row.UEmail,
			Grade:      row.UGrade,
			AuthStatus: row.UAuthStatus,
			AvatarUrl:  row.UAvatarUrl,
			SchoolID:   row.USchoolID,
			MajorID:    row.UMajorID,
			SchoolName: row.USchoolName,
			SchoolCode: row.USchoolCode,
			MajorName:  row.UMajorName,
			ClassID:    row.UClassID,
		}
		records = append(records, ob)
	}
	rows.Close()

	if err := r.enrichSkills(ctx, records, func(ob *models.OliveBranch) *models.User { return ob.Receiver }); err != nil {
		return nil, 0, err
	}

	return records, total, nil
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
