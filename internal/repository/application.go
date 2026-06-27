package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

// ErrNotProjectOwner is returned when the caller is not the owner of the target project.
var ErrNotProjectOwner = errors.New("not project owner")

// ApplicationRepository handles project application database operations
type ApplicationRepository struct {
	db *sqlx.DB
}

// ApplicationDashboardStats contains compact application metrics for dashboards.
type ApplicationDashboardStats struct {
	Total     int
	Read      int
	Approved  int
	Processed int
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
	ID                 int      `db:"id"`
	OpenID             string   `db:"openid"`
	Nickname           *string  `db:"nickname"`
	Phone              *string  `db:"phone"`
	Email              *string  `db:"email"`
	AvatarUrl          *string  `db:"avatar_url"`
	AuthStatus         *int     `db:"auth_status"`
	CollaborationScore *float64 `db:"collaboration_score"`
	Grade              *int     `db:"grade"`
	SchoolID           *int     `db:"school_id"`
	MajorID            *int     `db:"major_id"`
	SchoolName         *string  `db:"school_name"`
	SchoolCode         *string  `db:"school_code"`
	MajorName          *string  `db:"major_name"`
	ClassID            *int     `db:"class_id"`
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
			pa.status, pa.is_read, pa.reviewer_id, pa.reviewer_role, pa.assigned_role,
			pa.applied_at, pa.discussing_at, pa.rejected_at, pa.joined_at, pa.updated_at,
			p.name AS project_name,
			pr.name AS reviewer_role_name, ar.name AS assigned_role_name,
			CASE WHEN pm.id IS NULL THEN FALSE ELSE TRUE END AS is_current_member
		FROM project_application pa
		LEFT JOIN project p ON pa.project_id = p.id
		LEFT JOIN project_role pr ON pr.code = pa.reviewer_role
		LEFT JOIN project_role ar ON ar.code = pa.assigned_role
		LEFT JOIN project_members pm ON pm.project_id = pa.project_id AND pm.user_id = pa.user_id
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
			u.auth_status, u.collaboration_score, u.grade, u.school_id, u.major_id,
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
			ID:                 row.ID,
			OpenID:             row.OpenID,
			Nickname:           row.Nickname,
			Phone:              row.Phone,
			Email:              row.Email,
			AvatarUrl:          row.AvatarUrl,
			AuthStatus:         row.AuthStatus,
			CollaborationScore: row.CollaborationScore,
			Grade:              row.Grade,
			SchoolID:           row.SchoolID,
			MajorID:            row.MajorID,
			SchoolName:         row.SchoolName,
			SchoolCode:         row.SchoolCode,
			MajorName:          row.MajorName,
			ClassID:            row.ClassID,
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
			pa.status, pa.is_read, pa.reviewer_id, pa.reviewer_role, pa.assigned_role,
			pa.applied_at, pa.discussing_at, pa.rejected_at, pa.joined_at, pa.updated_at
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

// GetUnreadApplicationCount returns the number of the user's applications whose
// updated_at is strictly after applications_last_viewed_at. When the timestamp is
// nil (user never visited the page), all applications with status != 0 are counted.
// The result is capped at 99.
func (r *ApplicationRepository) GetUnreadApplicationCount(ctx context.Context, userID int) (int, error) {
	// Fetch last-viewed timestamp directly — avoids a heavy JOIN query in the handler.
	var viewedAt *time.Time
	if err := r.db.QueryRowxContext(ctx,
		"SELECT applications_last_viewed_at FROM `user` WHERE id = ?",
		userID,
	).Scan(&viewedAt); err != nil && err != sql.ErrNoRows {
		return 0, fmt.Errorf("get applications_last_viewed_at: %w", err)
	}

	var count int
	if viewedAt == nil {
		// Never viewed: count every application that is no longer in initial PENDING state
		if err := r.db.QueryRowxContext(ctx,
			`SELECT COUNT(*) FROM project_application WHERE user_id = ? AND status != 0`,
			userID,
		).Scan(&count); err != nil {
			return 0, fmt.Errorf("count unread applications (never viewed): %w", err)
		}
	} else {
		// Count applications updated after the last-viewed timestamp
		if err := r.db.QueryRowxContext(ctx,
			`SELECT COUNT(*) FROM project_application WHERE user_id = ? AND updated_at > ?`,
			userID, *viewedAt,
		).Scan(&count); err != nil {
			return 0, fmt.Errorf("count unread applications: %w", err)
		}
	}
	if count > 99 {
		count = 99
	}
	return count, nil
}

// MarkReviewerRead sets is_read = TRUE for applications belonging to projectID.
// reviewerID must be the project's creator or a project member.
// If ids is non-empty, only those specific records are updated; otherwise all unread records for the project are updated.
func (r *ApplicationRepository) MarkReviewerRead(ctx context.Context, projectID, reviewerID int, ids []int) error {
	var allowed bool
	if err := r.db.QueryRowxContext(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM project WHERE id = ? AND creator_id = ?
			UNION
			SELECT 1 FROM project_members WHERE project_id = ? AND user_id = ?
		)`,
		projectID, reviewerID, projectID, reviewerID,
	).Scan(&allowed); err != nil {
		return fmt.Errorf("check project reviewer: %w", err)
	}
	if !allowed {
		log.Printf("mark reviewer application read denied: projectID=%d reviewerID=%d", projectID, reviewerID)
		return ErrNotProjectOwner
	}

	var rowsAffected int64
	if len(ids) > 0 {
		query, args, err := sqlx.In(
			`UPDATE project_application SET is_read = TRUE WHERE project_id = ? AND id IN (?) AND is_read = FALSE`,
			projectID, ids,
		)
		if err != nil {
			return fmt.Errorf("build mark reviewer read IN query: %w", err)
		}
		query = r.db.Rebind(query)
		result, err := r.db.ExecContext(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("mark reviewer read: %w", err)
		}
		rowsAffected, _ = result.RowsAffected()
	} else {
		result, err := r.db.ExecContext(ctx,
			`UPDATE project_application SET is_read = TRUE WHERE project_id = ? AND is_read = FALSE`,
			projectID,
		)
		if err != nil {
			return fmt.Errorf("mark reviewer read: %w", err)
		}
		rowsAffected, _ = result.RowsAffected()
	}
	log.Printf("mark reviewer application read updated: projectID=%d reviewerID=%d ids=%v rowsAffected=%d", projectID, reviewerID, ids, rowsAffected)
	return nil
}

func (r *ApplicationRepository) GetProjectDashboardStats(ctx context.Context, projectID int) (ApplicationDashboardStats, error) {
	var stats ApplicationDashboardStats
	err := r.db.QueryRowxContext(ctx, `
		SELECT
			COUNT(DISTINCT user_id) AS total,
			COALESCE(SUM(CASE WHEN is_read = TRUE THEN 1 ELSE 0 END), 0) AS read_count,
			COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0) AS approved,
			COALESCE(SUM(CASE WHEN status <> ? THEN 1 ELSE 0 END), 0) AS processed
		FROM project_application
		WHERE project_id = ?
	`, models.ApplicationStatusJoined, models.ApplicationStatusPending, projectID).Scan(
		&stats.Total,
		&stats.Read,
		&stats.Approved,
		&stats.Processed,
	)
	if err != nil {
		return stats, fmt.Errorf("get project application dashboard stats: %w", err)
	}
	return stats, nil
}

func (r *ApplicationRepository) GetUserDashboardStats(ctx context.Context, userID int) (ApplicationDashboardStats, error) {
	var stats ApplicationDashboardStats
	err := r.db.QueryRowxContext(ctx, `
		SELECT
			COUNT(*) AS total,
			COALESCE(SUM(CASE WHEN is_read = TRUE THEN 1 ELSE 0 END), 0) AS read_count,
			COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0) AS approved,
			COALESCE(SUM(CASE WHEN status <> ? THEN 1 ELSE 0 END), 0) AS processed
		FROM project_application
		WHERE user_id = ?
	`, models.ApplicationStatusJoined, models.ApplicationStatusPending, userID).Scan(
		&stats.Total,
		&stats.Read,
		&stats.Approved,
		&stats.Processed,
	)
	if err != nil {
		return stats, fmt.Errorf("get user application dashboard stats: %w", err)
	}
	return stats, nil
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

func (r *ApplicationRepository) UpdateStatusWithReviewer(ctx context.Context, id int, status int, reviewerID int, reviewerRole *string) error {
	query := `UPDATE project_application
		SET status = ?, reviewer_id = ?, reviewer_role = ?,
			discussing_at = CASE WHEN ? = ? THEN CURRENT_TIMESTAMP ELSE discussing_at END,
			rejected_at = CASE WHEN ? = ? THEN CURRENT_TIMESTAMP ELSE rejected_at END,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query,
		status, reviewerID, reviewerRole,
		status, models.ApplicationStatusDiscussing,
		status, models.ApplicationStatusRejected,
		id,
	)
	if err != nil {
		return fmt.Errorf("update application status with reviewer: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("application not found")
	}

	return nil
}
