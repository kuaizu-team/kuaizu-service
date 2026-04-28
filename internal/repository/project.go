package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

// ProjectRepository handles project database operations
type ProjectRepository struct {
	db *sqlx.DB
}

// NewProjectRepository creates a new ProjectRepository
func NewProjectRepository(db *sqlx.DB) *ProjectRepository {
	return &ProjectRepository{db: db}
}

// ListParams contains parameters for listing projects
type ListParams struct {
	Page          int
	Size          int
	Keyword       *string
	SchoolID      *int
	Status        *int
	Statuses      []int
	Direction     *int
	CreatorID     *int
	IsCrossSchool *int
}

// List retrieves paginated projects with optional filters
func (r *ProjectRepository) List(ctx context.Context, params ListParams) ([]models.Project, int64, error) {
	conditions := []string{"1=1"}
	args := []interface{}{}

	if params.Keyword != nil && *params.Keyword != "" {
		conditions = append(conditions, "(p.name LIKE ? OR p.description LIKE ?)")
		args = append(args, "%"+*params.Keyword+"%", "%"+*params.Keyword+"%")
	}
	if params.SchoolID != nil {
		conditions = append(conditions, "p.school_id = ?")
		args = append(args, *params.SchoolID)
	}
	if len(params.Statuses) > 0 {
		placeholders := make([]string, len(params.Statuses))
		for i, status := range params.Statuses {
			placeholders[i] = "?"
			args = append(args, status)
		}
		conditions = append(conditions, fmt.Sprintf("p.status IN (%s)", strings.Join(placeholders, ",")))
	} else if params.Status != nil {
		conditions = append(conditions, "p.status = ?")
		args = append(args, *params.Status)
	}
	if params.Direction != nil {
		conditions = append(conditions, "p.direction = ?")
		args = append(args, *params.Direction)
	}
	if params.CreatorID != nil {
		conditions = append(conditions, "p.creator_id = ?")
		args = append(args, *params.CreatorID)
	}
	if params.IsCrossSchool != nil {
		conditions = append(conditions, "p.is_cross_school = ?")
		args = append(args, *params.IsCrossSchool)
	}

	whereClause := strings.Join(conditions, " AND ")

	// Count total
	var total int64
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM project p WHERE %s`, whereClause)
	if err := r.db.QueryRowxContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count projects: %w", err)
	}

	// Query with pagination — column aliases match Project db tags
	offset := (params.Page - 1) * params.Size
	query := fmt.Sprintf(`
		SELECT
			p.id, p.creator_id, p.name, p.description, p.school_id,
			p.direction, p.member_count, p.status,
			p.promotion_status, p.promotion_expire_time, p.view_count,
			p.created_at, p.updated_at, p.is_cross_school,
			p.education_requirement, p.skill_requirement,
			s.school_name
		FROM project p
		LEFT JOIN school s ON p.school_id = s.id
		WHERE %s
		ORDER BY p.created_at DESC
		LIMIT ? OFFSET ?
	`, whereClause)
	args = append(args, params.Size, offset)

	var projects []models.Project
	if err := r.db.SelectContext(ctx, &projects, query, args...); err != nil {
		return nil, 0, fmt.Errorf("query projects: %w", err)
	}

	return projects, total, nil
}

// creatorRow holds the JOIN-ed creator columns for GetByID.
// Column aliases (u_*) avoid conflicts with project columns of the same name.
type creatorRow struct {
	UID                  int        `db:"u_id"`
	UOpenID              string     `db:"u_openid"`
	UNickname            *string    `db:"u_nickname"`
	UPhone               *string    `db:"u_phone"`
	UEmail               *string    `db:"u_email"`
	UAuthStatus          *int       `db:"u_auth_status"`
	UAvatarUrl           *string    `db:"u_avatar_url"`
	UCreatedAt           *time.Time `db:"u_created_at"`
	USchoolName          *string    `db:"u_school_name"`
	UTalentProfileStatus *int       `db:"u_talent_profile_status"`
}

// projectRow is the flat scan target for GetByID (project + creator columns).
type projectRow struct {
	models.Project
	creatorRow
}

// GetByID retrieves a project by ID with creator info
func (r *ProjectRepository) GetByID(ctx context.Context, id int) (*models.Project, error) {
	query := `
		SELECT
			p.id, p.creator_id, p.name, p.description, p.school_id,
			p.direction, p.member_count, p.status,
			p.promotion_status, p.promotion_expire_time, p.view_count,
			p.created_at, p.updated_at, p.is_cross_school,
			p.education_requirement, p.skill_requirement,
			s.school_name,
			u.id          AS u_id,
			u.openid      AS u_openid,
			u.nickname    AS u_nickname,
			u.phone       AS u_phone,
			u.email       AS u_email,
			u.auth_status AS u_auth_status,
			u.avatar_url  AS u_avatar_url,
			u.created_at  AS u_created_at,
			us.school_name AS u_school_name,
			tp.status      AS u_talent_profile_status
		FROM project p
		LEFT JOIN school s ON p.school_id = s.id
		LEFT JOIN ` + "`user`" + ` u ON p.creator_id = u.id
		LEFT JOIN school us ON u.school_id = us.id
		LEFT JOIN talent_profile tp ON u.id = tp.user_id
		WHERE p.id = ?
	`

	var row projectRow
	if err := r.db.QueryRowxContext(ctx, query, id).StructScan(&row); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query project by id: %w", err)
	}

	p := row.Project
	p.Creator = &models.User{
		ID:         row.UID,
		OpenID:     row.UOpenID,
		Nickname:   row.UNickname,
		Phone:      row.UPhone,
		Email:      row.UEmail,
		AuthStatus: row.UAuthStatus,
		AvatarUrl:  row.UAvatarUrl,
		CreatedAt:  row.UCreatedAt,
		SchoolName: row.USchoolName,
	}
	p.CreatorTalentProfileStatus = row.UTalentProfileStatus
	return &p, nil
}

// Create creates a new project
func (r *ProjectRepository) Create(ctx context.Context, p *models.Project) error {
	query := `
		INSERT INTO project (
			creator_id, name, description, school_id, direction,
			member_count, status, promotion_status, view_count,
			is_cross_school, education_requirement, skill_requirement
		) VALUES (
			:creator_id, :name, :description, :school_id, :direction,
			:member_count, :status, :promotion_status, :view_count,
			:is_cross_school, :education_requirement, :skill_requirement
		)
	`

	result, err := r.db.NamedExecContext(ctx, query, p)
	if err != nil {
		return fmt.Errorf("create project: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	p.ID = int(id)
	return nil
}

// Update updates a project
func (r *ProjectRepository) Update(ctx context.Context, p *models.Project) error {
	query := `
		UPDATE project SET
			name                 = :name,
			description          = :description,
			direction            = :direction,
			member_count         = :member_count,
			is_cross_school      = :is_cross_school,
			education_requirement = :education_requirement,
			skill_requirement    = :skill_requirement,
			updated_at           = CURRENT_TIMESTAMP
		WHERE id = :id
	`

	result, err := r.db.NamedExecContext(ctx, query, p)
	if err != nil {
		return fmt.Errorf("update project: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("project not found")
	}
	return nil
}

// Delete performs a logical delete (sets status to CLOSED)
func (r *ProjectRepository) Delete(ctx context.Context, id int) error {
	query := `UPDATE project SET status = 3, updated_at = CURRENT_TIMESTAMP WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("project not found")
	}
	return nil
}

// IsOwner checks if a user is the creator of a project
func (r *ProjectRepository) IsOwner(ctx context.Context, projectID, userID int) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM project WHERE id = ? AND creator_id = ?)`
	if err := r.db.QueryRowxContext(ctx, query, projectID, userID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check project owner: %w", err)
	}
	return exists, nil
}

// UpdateStatus updates the review status of a project
func (r *ProjectRepository) UpdateStatus(ctx context.Context, id int, status int) error {
	query := `UPDATE project SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("update project status: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("project not found")
	}
	return nil
}

// IncrementViewCount increments the view count of a project
func (r *ProjectRepository) IncrementViewCount(ctx context.Context, id int) error {
	query := `UPDATE project SET view_count = view_count + 1 WHERE id = ?`
	if _, err := r.db.ExecContext(ctx, query, id); err != nil {
		return fmt.Errorf("increment view count: %w", err)
	}
	return nil
}
