package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/trv3wood/kuaizu-server/internal/models"
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
	Page      int
	Size      int
	Keyword   *string
	SchoolID  *int
	Status    *int
	Direction *int
	CreatorID *int
}

// List retrieves paginated projects with optional filters
func (r *ProjectRepository) List(ctx context.Context, params ListParams) ([]models.Project, int64, error) {
	// Build WHERE clause
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

	if params.Status != nil {
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

	whereClause := strings.Join(conditions, " AND ")

	// Count total
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM project p WHERE %s`, whereClause)
	var total int64
	err := r.db.QueryRowxContext(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count projects: %w", err)
	}

	// Query with pagination
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

	rows, err := r.db.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query projects: %w", err)
	}
	defer rows.Close()

	var projects []models.Project
	for rows.Next() {
		var p models.Project
		err := rows.Scan(
			&p.ID, &p.CreatorID, &p.Name, &p.Description, &p.SchoolID,
			&p.Direction, &p.MemberCount, &p.Status,
			&p.PromotionStatus, &p.PromotionExpireTime, &p.ViewCount,
			&p.CreatedAt, &p.UpdatedAt, &p.IsCrossSchool,
			&p.EducationRequirement, &p.SkillRequirement,
			&p.SchoolName,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, p)
	}

	return projects, total, nil
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
			u.id, u.openid, u.nickname, u.phone, u.email,
			u.auth_status, u.avatar_url, u.created_at
		FROM project p
		LEFT JOIN school s ON p.school_id = s.id
		LEFT JOIN ` + "`user`" + ` u ON p.creator_id = u.id
		WHERE p.id = ?
	`

	var p models.Project
	var creator models.User
	err := r.db.QueryRowxContext(ctx, query, id).Scan(
		&p.ID, &p.CreatorID, &p.Name, &p.Description, &p.SchoolID,
		&p.Direction, &p.MemberCount, &p.Status,
		&p.PromotionStatus, &p.PromotionExpireTime, &p.ViewCount,
		&p.CreatedAt, &p.UpdatedAt, &p.IsCrossSchool,
		&p.EducationRequirement, &p.SkillRequirement,
		&p.SchoolName,
		&creator.ID, &creator.OpenID, &creator.Nickname, &creator.Phone, &creator.Email,
		&creator.AuthStatus, &creator.AvatarUrl, &creator.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query project by id: %w", err)
	}

	p.Creator = &creator
	return &p, nil
}

// Create creates a new project
func (r *ProjectRepository) Create(ctx context.Context, p *models.Project) error {
	query := `
		INSERT INTO project (
			creator_id, name, description, school_id, direction,
			member_count, status, promotion_status, view_count,
			is_cross_school, education_requirement, skill_requirement
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := r.db.ExecContext(ctx, query,
		p.CreatorID, p.Name, p.Description, p.SchoolID, p.Direction,
		p.MemberCount, p.Status, p.PromotionStatus, p.ViewCount,
		p.IsCrossSchool, p.EducationRequirement, p.SkillRequirement,
	)
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
			name = ?,
			description = ?,
			direction = ?,
			member_count = ?,
			is_cross_school = ?,
			education_requirement = ?,
			skill_requirement = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`

	result, err := r.db.ExecContext(ctx, query,
		p.Name, p.Description, p.Direction, p.MemberCount,
		p.IsCrossSchool, p.EducationRequirement, p.SkillRequirement,
		p.ID,
	)
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
	query := `SELECT EXISTS(SELECT 1 FROM project WHERE id = ? AND creator_id = ?)`
	var exists bool
	err := r.db.QueryRowxContext(ctx, query, projectID, userID).Scan(&exists)
	if err != nil {
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
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("increment view count: %w", err)
	}
	return nil
}
