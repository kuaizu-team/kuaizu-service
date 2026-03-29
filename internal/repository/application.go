package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/trv3wood/kuaizu-server/internal/models"
)

// ApplicationRepository handles project application database operations
type ApplicationRepository struct {
	db *sqlx.DB
}

// NewApplicationRepository creates a new ApplicationRepository
func NewApplicationRepository(db *sqlx.DB) *ApplicationRepository {
	return &ApplicationRepository{db: db}
}

// ApplicationListParams contains parameters for listing applications
type ApplicationListParams struct {
	UserID    *int // applicant id
	Page      int
	Size      int
	ProjectID *int
	Status    *int
}

// userWithSchoolMajor holds user + school + major columns for the second batch query.
type userWithSchoolMajor struct {
	ID         int     `db:"id"`
	OpenID     string  `db:"openid"`
	Nickname   *string `db:"nickname"`
	Phone      *string `db:"phone"`
	Email      *string `db:"email"`
	AvatarUrl  *string `db:"avatar_url"`
	SchoolID   *int    `db:"school_id"`
	MajorID    *int    `db:"major_id"`
	SchoolName *string `db:"school_name"`
	SchoolCode *string `db:"school_code"`
	MajorName  *string `db:"major_name"`
	ClassID    *int    `db:"class_id"`
}

// talentProfileRow holds talent_profile columns for the third batch query.
type talentProfileRow struct {
	ID           int                    `db:"id"`
	UserID       int                    `db:"user_id"`
	SkillSummary models.JSONStringArray `db:"skill_summary"`
}

// List retrieves paginated applications for a project with applicant info
func (r *ApplicationRepository) List(ctx context.Context, params ApplicationListParams) ([]models.ProjectApplication, int64, error) {
	// Build WHERE clause
	conditions := []string{}
	args := []interface{}{}

	if params.ProjectID != nil {
		conditions = append(conditions, "pa.project_id = ?")
		args = append(args, *params.ProjectID)
	}

	if params.Status != nil {
		conditions = append(conditions, "pa.status = ?")
		args = append(args, *params.Status)
	}

	if params.UserID != nil {
		conditions = append(conditions, "pa.user_id = ?")
		args = append(args, *params.UserID)
	}

	whereClause := strings.Join(conditions, " AND ")

	// Count total
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM project_application pa WHERE %s`, whereClause)
	var total int64
	if err := r.db.QueryRowxContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count applications: %w", err)
	}

	// 1st query: project_application + project (2 tables)
	offset := (params.Page - 1) * params.Size
	query := fmt.Sprintf(`
		SELECT
			pa.id, pa.project_id, pa.user_id,
			pa.status, pa.applied_at, pa.updated_at,
			p.name AS project_name
		FROM project_application pa
		LEFT JOIN project p ON pa.project_id = p.id
		WHERE %s
		ORDER BY pa.applied_at DESC
		LIMIT ? OFFSET ?
	`, whereClause)
	args = append(args, params.Size, offset)

	var applications []models.ProjectApplication
	if err := r.db.SelectContext(ctx, &applications, query, args...); err != nil {
		return nil, 0, fmt.Errorf("query applications: %w", err)
	}

	if len(applications) == 0 {
		return applications, total, nil
	}

	// 2nd query: user + school + major (3 tables), batch by user_id
	userIDs := make([]int, 0, len(applications))
	for _, a := range applications {
		userIDs = append(userIDs, a.UserID)
	}
	userQuery, userArgs, err := sqlx.In(`
		SELECT
			u.id, u.openid, u.nickname, u.phone, u.email, u.avatar_url,
			u.school_id, u.major_id,
			s.school_name, s.school_code,
			m.major_name, m.class_id
		FROM `+"`user`"+` u
		LEFT JOIN school s ON u.school_id = s.id
		LEFT JOIN major m ON u.major_id = m.id
		WHERE u.id IN (?)
	`, userIDs)
	if err != nil {
		return nil, 0, fmt.Errorf("build user+school+major IN query: %w", err)
	}
	userQuery = r.db.Rebind(userQuery)

	var userRows []userWithSchoolMajor
	if err := r.db.SelectContext(ctx, &userRows, userQuery, userArgs...); err != nil {
		return nil, 0, fmt.Errorf("batch query user+school+major: %w", err)
	}

	// Build user lookup map
	userMap := make(map[int]*models.User, len(userRows))
	for _, row := range userRows {
		userMap[row.ID] = &models.User{
			ID:         row.ID,
			OpenID:     row.OpenID,
			Nickname:   row.Nickname,
			Phone:      row.Phone,
			Email:      row.Email,
			AvatarUrl:  row.AvatarUrl,
			SchoolID:   row.SchoolID,
			MajorID:    row.MajorID,
			SchoolName: row.SchoolName,
			SchoolCode: row.SchoolCode,
			MajorName:  row.MajorName,
			ClassID:    row.ClassID,
		}
	}

	// 3rd query: talent_profile (1 table), batch by user_id
	tpQuery, tpArgs, err := sqlx.In(`
		SELECT id, user_id, skill_summary
		FROM talent_profile
		WHERE user_id IN (?)
	`, userIDs)
	if err != nil {
		return nil, 0, fmt.Errorf("build talent_profile IN query: %w", err)
	}
	tpQuery = r.db.Rebind(tpQuery)

	var tpRows []talentProfileRow
	if err := r.db.SelectContext(ctx, &tpRows, tpQuery, tpArgs...); err != nil {
		return nil, 0, fmt.Errorf("batch query talent_profile: %w", err)
	}

	// Build talent_profile lookup map
	tpMap := make(map[int]*models.TalentProfile, len(tpRows))
	for _, row := range tpRows {
		tpMap[row.UserID] = &models.TalentProfile{
			ID:           row.ID,
			UserID:       row.UserID,
			SkillSummary: row.SkillSummary,
		}
	}

	// Fill back applicant and talent_profile
	for i := range applications {
		if user, ok := userMap[applications[i].UserID]; ok {
			applications[i].Applicant = user
		}
		if tp, ok := tpMap[applications[i].UserID]; ok {
			applications[i].TalentProfile = tp
		}
	}

	return applications, total, nil
}

// Create creates a new application
func (r *ApplicationRepository) Create(ctx context.Context, app *models.ProjectApplication) error {
	query := `
		INSERT INTO project_application (
			project_id, user_id, status
		) VALUES (
			:project_id, :user_id, :status
		)
	`

	result, err := r.db.NamedExecContext(ctx, query, app)
	if err != nil {
		return fmt.Errorf("create application: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	app.ID = int(id)

	return nil
}

// GetByID retrieves an application by ID
func (r *ApplicationRepository) GetByID(ctx context.Context, id int) (*models.ProjectApplication, error) {
	query := `
		SELECT
			pa.id, pa.project_id, pa.user_id,
			pa.status, pa.applied_at, pa.updated_at
		FROM project_application pa
		WHERE pa.id = ?
	`

	var app models.ProjectApplication
	if err := r.db.QueryRowxContext(ctx, query, id).StructScan(&app); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query application by id: %w", err)
	}

	return &app, nil
}

// CheckDuplicate checks if a user has already applied to a project
func (r *ApplicationRepository) CheckDuplicate(ctx context.Context, projectID, userID int) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM project_application WHERE project_id = ? AND user_id = ?)`
	var exists bool
	if err := r.db.QueryRowxContext(ctx, query, projectID, userID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check duplicate application: %w", err)
	}
	return exists, nil
}

// UpdateStatus updates the status and reply message of an application
func (r *ApplicationRepository) UpdateStatus(ctx context.Context, id int, status int) error {
	query := `UPDATE project_application SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("update application status: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("application not found")
	}

	return nil
}
