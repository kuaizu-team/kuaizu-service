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
	SortBy        *string // "school_priority" enables priority ordering
	UserSchoolID  *int    // used when SortBy == "school_priority"

	// Geo info of the user's school — pre-fetched by the service layer.
	// Used to build P2/P3/P4 tiers in school_priority ordering.
	UserSchoolProvince *string
	UserSchoolCity     *string
	UserSchoolDistrict *string
}

// List retrieves paginated projects with optional filters
func (r *ProjectRepository) List(ctx context.Context, params ListParams) ([]models.Project, int64, error) {
	conditions := []string{"1=1"}
	whereArgs := []interface{}{}

	if params.Keyword != nil && *params.Keyword != "" {
		conditions = append(conditions, "(p.name LIKE ? OR p.description LIKE ?)")
		whereArgs = append(whereArgs, "%"+*params.Keyword+"%", "%"+*params.Keyword+"%")
	}
	if params.SchoolID != nil {
		conditions = append(conditions, "p.school_id = ?")
		whereArgs = append(whereArgs, *params.SchoolID)
	}
	if len(params.Statuses) > 0 {
		placeholders := make([]string, len(params.Statuses))
		for i, status := range params.Statuses {
			placeholders[i] = "?"
			whereArgs = append(whereArgs, status)
		}
		conditions = append(conditions, fmt.Sprintf("p.status IN (%s)", strings.Join(placeholders, ",")))
	} else if params.Status != nil {
		conditions = append(conditions, "p.status = ?")
		whereArgs = append(whereArgs, *params.Status)
	}
	if params.Direction != nil {
		conditions = append(conditions, "p.direction = ?")
		whereArgs = append(whereArgs, *params.Direction)
	}
	if params.CreatorID != nil {
		conditions = append(conditions, "p.creator_id = ?")
		whereArgs = append(whereArgs, *params.CreatorID)
	}
	if params.IsCrossSchool != nil {
		conditions = append(conditions, "p.is_cross_school = ?")
		whereArgs = append(whereArgs, *params.IsCrossSchool)
	}

	whereClause := strings.Join(conditions, " AND ")

	// Count total (WHERE args only — no ORDER BY or pagination args needed here)
	var total int64
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM project p WHERE %s`, whereClause)
	if err := r.db.QueryRowxContext(ctx, countQuery, whereArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count projects: %w", err)
	}

	// Build ORDER BY clause and its own placeholder args (separate from whereArgs so
	// they can be placed after the WHERE args but before LIMIT/OFFSET).
	//
	// Modes:
	//   "updated_at"      → p.updated_at DESC  (used by ListMyProjects)
	//   "school_priority" → 5-tier geo priority + cross-school sub-sort + created_at tiebreak
	//   default           → p.created_at DESC
	orderClause := "p.created_at DESC"
	var orderArgs []interface{}

	if params.SortBy != nil && *params.SortBy == "updated_at" {
		orderClause = "p.updated_at DESC"
	} else if params.SortBy != nil && *params.SortBy == "school_priority" {
		schoolID := 0
		if params.UserSchoolID != nil {
			schoolID = *params.UserSchoolID
		}
		if schoolID != 0 {
			// --- Build 5-tier priority CASE WHEN ---
			// P1: same school
			// P2: same district + city (skipped if user school has no district)
			// P3: same city
			// P4: same province
			// P5: everything else
			//
			// Geo comparison is against the PROJECT's school columns (s.*) from the
			// existing LEFT JOIN school s ON p.school_id = s.id.
			var tierWHENs []string

			// P1 — same school (no geo join needed)
			tierWHENs = append(tierWHENs, "WHEN p.school_id = ? THEN 1")
			orderArgs = append(orderArgs, schoolID)

			// P2 — same district (only when user school has a valid district)
			if params.UserSchoolDistrict != nil && *params.UserSchoolDistrict != "" &&
				params.UserSchoolCity != nil && *params.UserSchoolCity != "" {
				tierWHENs = append(tierWHENs, "WHEN s.district = ? AND s.city = ? THEN 2")
				orderArgs = append(orderArgs, *params.UserSchoolDistrict, *params.UserSchoolCity)
			}

			// P3 — same city
			if params.UserSchoolCity != nil && *params.UserSchoolCity != "" {
				tierWHENs = append(tierWHENs, "WHEN s.city = ? THEN 3")
				orderArgs = append(orderArgs, *params.UserSchoolCity)
			}

			// P4 — same province
			if params.UserSchoolProvince != nil && *params.UserSchoolProvince != "" {
				tierWHENs = append(tierWHENs, "WHEN s.province = ? THEN 4")
				orderArgs = append(orderArgs, *params.UserSchoolProvince)
			}

			tierExpr := "CASE\n" + strings.Join(tierWHENs, "\n") + "\nELSE 5\nEND"

			// Cross-school sub-sort within each tier:
			//   P1 rows → sub-sort 0 (no distinction; every P1 project is already "same school")
			//   Other tiers → is_cross_school=1 sorts before is_cross_school=0
			//     (1 - COALESCE(is_cross_school,0)): cross=1→0, cross=0→1
			const crossExpr = "CASE WHEN p.school_id = ? THEN 0 ELSE (1 - COALESCE(p.is_cross_school, 0)) END"
			orderArgs = append(orderArgs, schoolID)

			orderClause = fmt.Sprintf("%s ASC, %s ASC, p.created_at DESC", tierExpr, crossExpr)
		}
		// If schoolID == 0 (no user school context), fall through to default created_at DESC
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
			s.school_name,
			COALESCE(pa_counts.pending_count, 0) AS pending_application_count
		FROM project p
		LEFT JOIN school s ON p.school_id = s.id
		LEFT JOIN (
			SELECT project_id, COUNT(*) AS pending_count
			FROM project_application
			WHERE status = 0
			GROUP BY project_id
		) pa_counts ON pa_counts.project_id = p.id
		WHERE %s
		ORDER BY %s
		LIMIT ? OFFSET ?
	`, whereClause, orderClause)

	// Combine: WHERE args → ORDER BY args → LIMIT/OFFSET args
	dataArgs := make([]interface{}, 0, len(whereArgs)+len(orderArgs)+2)
	dataArgs = append(dataArgs, whereArgs...)
	dataArgs = append(dataArgs, orderArgs...)
	dataArgs = append(dataArgs, params.Size, offset)

	var projects []models.Project
	if err := r.db.SelectContext(ctx, &projects, query, dataArgs...); err != nil {
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
	UWechatID            *string    `db:"u_wechat_id"`
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
			u.wechat_id   AS u_wechat_id,
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
		WechatID:   row.UWechatID,
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
