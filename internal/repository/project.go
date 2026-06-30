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
	EventID       *int
	CreatorID     *int
	MemberUserID  *int
	IsCrossSchool *int
	// SortBy controls the primary sort key.
	// Public values: "school_priority" (geo-priority), "updated_at"
	// Admin values:  "pendingCount" (combined admin pending count), "createdAt"
	SortBy *string
	// Order controls asc/desc direction for "pendingCount" and "createdAt" sortBy.
	// Values: "asc" | "desc" (case-insensitive). Defaults to DESC.
	Order        *string
	UserSchoolID *int // used when SortBy == "school_priority"

	// Geo info of the user's school — pre-fetched by the service layer.
	// Used to build P2/P3/P4 tiers in school_priority ordering.
	UserSchoolProvince *string
	UserSchoolCity     *string
	UserSchoolDistrict *string

	// IncludePendingCount — admin-only flag.
	// When true, adds a pending_count column to SELECT (sum of pending applications
	// and pending olive branches for each project). Required for SortBy="pendingCount".
	IncludePendingCount bool
	// RandomSeed keeps within-tier ordering stable for one pagination session.
	RandomSeed string
}

// List retrieves paginated projects with optional filters
func (r *ProjectRepository) List(ctx context.Context, params ListParams) ([]models.Project, int64, error) {
	conditions := []string{"1=1"}
	whereArgs := []interface{}{}

	if params.Keyword != nil && *params.Keyword != "" {
		conditions = append(conditions, `(p.name LIKE ? OR p.description LIKE ?
			OR EXISTS (SELECT 1 FROM school ks WHERE ks.id=p.school_id AND ks.school_name LIKE ?)
			OR EXISTS (SELECT 1 FROM project_tag_relation ptr JOIN project_tag pt ON pt.id=ptr.tag_id
				WHERE ptr.project_id=p.id AND pt.status=1 AND pt.name LIKE ?))`)
		pattern := "%" + *params.Keyword + "%"
		whereArgs = append(whereArgs, pattern, pattern, pattern, pattern)
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
	if params.EventID != nil {
		conditions = append(conditions, "EXISTS (SELECT 1 FROM project_event pe WHERE pe.project_id = p.id AND pe.event_id = ?)")
		whereArgs = append(whereArgs, *params.EventID)
	}
	if params.CreatorID != nil {
		conditions = append(conditions, "p.creator_id = ?")
		whereArgs = append(whereArgs, *params.CreatorID)
	}
	if params.MemberUserID != nil {
		conditions = append(conditions, `(p.creator_id = ? OR EXISTS (
			SELECT 1 FROM project_members pm
			WHERE pm.project_id = p.id AND pm.user_id = ?
		))`)
		whereArgs = append(whereArgs, *params.MemberUserID, *params.MemberUserID)
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
	} else if params.SortBy != nil && *params.SortBy == "pendingCount" {
		// Admin sort by combined pending count.
		// IncludePendingCount must be true for pending_count to exist in SELECT.
		dir := "DESC"
		if params.Order != nil && strings.EqualFold(*params.Order, "asc") {
			dir = "ASC"
		}
		orderClause = fmt.Sprintf("pending_count %s, p.created_at DESC", dir)
	} else if params.SortBy != nil && *params.SortBy == "createdAt" {
		dir := "DESC"
		if params.Order != nil && strings.EqualFold(*params.Order, "asc") {
			dir = "ASC"
		}
		orderClause = fmt.Sprintf("p.created_at %s, p.id %s", dir, dir)
	} else if params.SortBy != nil && *params.SortBy == "id" {
		dir := "DESC"
		if params.Order != nil && strings.EqualFold(*params.Order, "asc") {
			dir = "ASC"
		}
		orderClause = fmt.Sprintf("p.id %s", dir)
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

			// Every 10 heat points (like + 2*favorite) promotes one tier.
			// The cap prevents promotion above P1; the request seed keeps
			// pagination stable and changes on an explicit refresh.
			const heatScoreExpr = "COALESCE(plc.like_count, 0) + COALESCE(pfc.favorite_count, 0) * 2"
			finalTierExpr := fmt.Sprintf("GREATEST(1, (%s) - FLOOR((%s) / 10))", tierExpr, heatScoreExpr)
			orderClause = fmt.Sprintf(`%s ASC,
				ROW_NUMBER() OVER (PARTITION BY p.creator_id ORDER BY CRC32(CONCAT(?, ':item:', p.id))) ASC,
				CRC32(CONCAT(?, ':owner:', p.creator_id)) ASC,
				CRC32(CONCAT(?, ':item:', p.id)) ASC, p.id ASC`, finalTierExpr)
			orderArgs = append(orderArgs, params.RandomSeed, params.RandomSeed, params.RandomSeed)
		}
		// If schoolID == 0 (no user school context), fall through to default created_at DESC
	}

	// Query with pagination — column aliases match Project db tags.
	// When IncludePendingCount=true, include olive-branch JOIN and compute
	// the combined pending_count = pending applications + pending olive branches.
	offset := (params.Page - 1) * params.Size

	pendingCountSelect := ""
	pendingCountJoin := ""
	heatJoin := ""
	if params.SortBy != nil && *params.SortBy == "school_priority" && params.UserSchoolID != nil && *params.UserSchoolID != 0 {
		heatJoin = `
		LEFT JOIN (
			SELECT project_id, COUNT(*) AS like_count
			FROM project_like
			GROUP BY project_id
		) plc ON plc.project_id = p.id
		LEFT JOIN (
			SELECT project_id, COUNT(*) AS favorite_count
			FROM project_favorite
			GROUP BY project_id
		) pfc ON pfc.project_id = p.id`
	}
	if params.IncludePendingCount {
		pendingCountSelect = `,
			COALESCE(pa_counts.pending_count, 0) + COALESCE(ob_counts.pending_count, 0) AS pending_count`
		pendingCountJoin = `
		LEFT JOIN (
			SELECT related_project_id, COUNT(*) AS pending_count
			FROM olive_branch_record
			WHERE status = 0
			GROUP BY related_project_id
		) ob_counts ON ob_counts.related_project_id = p.id`
	}

	query := fmt.Sprintf(`
		SELECT
			p.id, p.creator_id, p.name, p.description, p.school_id,
			p.direction, p.member_count, p.status,
			p.promotion_status, p.promotion_expire_time, p.view_count,
			p.created_at, p.updated_at, p.recruit_completed_at, p.ended_at, p.reject_reason, p.deleted_at, p.is_cross_school,
			p.education_requirement, p.skill_requirement,
			p.publisher_role, p.initiating_school_id,
			s.school_name, pr.name AS publisher_role_name, ins.school_name AS initiating_school_name,
			COALESCE(pa_counts.pending_count, 0) AS pending_application_count%s
		FROM project p
		LEFT JOIN school s ON p.school_id = s.id
		LEFT JOIN project_role pr ON p.publisher_role = pr.code
		LEFT JOIN school ins ON p.initiating_school_id = ins.id%s
		LEFT JOIN (
			SELECT project_id, COUNT(*) AS pending_count
			FROM project_application
			WHERE status = 0
			GROUP BY project_id
		) pa_counts ON pa_counts.project_id = p.id%s
		WHERE %s
		ORDER BY %s
		LIMIT ? OFFSET ?
	`, pendingCountSelect, heatJoin, pendingCountJoin, whereClause, orderClause)

	// Combine: WHERE args → ORDER BY args → LIMIT/OFFSET args
	dataArgs := make([]interface{}, 0, len(whereArgs)+len(orderArgs)+2)
	dataArgs = append(dataArgs, whereArgs...)
	dataArgs = append(dataArgs, orderArgs...)
	dataArgs = append(dataArgs, params.Size, offset)

	var projects []models.Project
	if err := r.db.SelectContext(ctx, &projects, query, dataArgs...); err != nil {
		return nil, 0, fmt.Errorf("query projects: %w", err)
	}
	if err := r.enrichTagsBatch(ctx, projects); err != nil {
		return nil, 0, err
	}
	if err := r.enrichCreatorsBatch(ctx, projects); err != nil {
		return nil, 0, err
	}

	return projects, total, nil
}

func (r *ProjectRepository) enrichCreatorsBatch(ctx context.Context, projects []models.Project) error {
	if len(projects) == 0 {
		return nil
	}
	ids, index := make([]int, 0, len(projects)), map[int][]int{}
	for i := range projects {
		if _, ok := index[projects[i].CreatorID]; !ok {
			ids = append(ids, projects[i].CreatorID)
		}
		index[projects[i].CreatorID] = append(index[projects[i].CreatorID], i)
	}
	query, args, err := sqlx.In(`SELECT u.id,u.openid,u.nickname,u.avatar_url,u.auth_status,u.collaboration_score,s.school_name,m.major_name,tp.id talent_profile_id
		FROM `+"`user`"+` u LEFT JOIN school s ON s.id=u.school_id LEFT JOIN major m ON m.id=u.major_id LEFT JOIN talent_profile tp ON tp.user_id=u.id WHERE u.id IN (?)`, ids)
	if err != nil {
		return err
	}
	var creators []models.User
	if err := r.db.SelectContext(ctx, &creators, r.db.Rebind(query), args...); err != nil {
		return fmt.Errorf("query project creators: %w", err)
	}
	for i := range creators {
		creators[i].Nickname = models.DisplayNickname(creators[i].Nickname)
		for _, projectIndex := range index[creators[i].ID] {
			creator := creators[i]
			projects[projectIndex].Creator = &creator
		}
	}
	return nil
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
	UTalentProfileID     *int       `db:"u_talent_profile_id"`
	UMajorName           *string    `db:"u_major_name"`
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
			p.created_at, p.updated_at, p.recruit_completed_at, p.ended_at, p.reject_reason, p.deleted_at, p.is_cross_school,
			p.education_requirement, p.skill_requirement,
			p.publisher_role, p.initiating_school_id,
			s.school_name, pr.name AS publisher_role_name, ins.school_name AS initiating_school_name,
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
			,tp.id         AS u_talent_profile_id
			,um.major_name AS u_major_name
		FROM project p
		LEFT JOIN school s ON p.school_id = s.id
		LEFT JOIN project_role pr ON p.publisher_role = pr.code
		LEFT JOIN school ins ON p.initiating_school_id = ins.id
		LEFT JOIN ` + "`user`" + ` u ON p.creator_id = u.id
		LEFT JOIN school us ON u.school_id = us.id
		LEFT JOIN talent_profile tp ON u.id = tp.user_id
		LEFT JOIN major um ON u.major_id = um.id
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
		ID:              row.UID,
		OpenID:          row.UOpenID,
		Nickname:        models.DisplayNickname(row.UNickname),
		Phone:           row.UPhone,
		Email:           row.UEmail,
		WechatID:        row.UWechatID,
		AuthStatus:      row.UAuthStatus,
		AvatarUrl:       row.UAvatarUrl,
		CreatedAt:       row.UCreatedAt,
		SchoolName:      row.USchoolName,
		TalentProfileID: row.UTalentProfileID,
		MajorName:       row.UMajorName,
	}
	p.CreatorTalentProfileStatus = row.UTalentProfileStatus
	items := []models.Project{p}
	if err := r.enrichTagsBatch(ctx, items); err != nil {
		return nil, err
	}
	p = items[0]
	return &p, nil
}

func (r *ProjectRepository) enrichTagsBatch(ctx context.Context, projects []models.Project) error {
	if len(projects) == 0 {
		return nil
	}
	ids := make([]int, len(projects))
	index := make(map[int]int, len(projects))
	for i := range projects {
		ids[i] = projects[i].ID
		index[projects[i].ID] = i
	}
	query, args, err := sqlx.In(`SELECT r.project_id,t.id,t.name FROM project_tag_relation r JOIN project_tag t ON t.id=r.tag_id WHERE r.project_id IN (?) AND t.status=1 ORDER BY t.sort_order,t.id`, ids)
	if err != nil {
		return err
	}
	rows, err := r.db.QueryxContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return fmt.Errorf("query project tags: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var projectID int
		var tag models.ProjectTag
		if err := rows.Scan(&projectID, &tag.ID, &tag.Name); err != nil {
			return err
		}
		if i, ok := index[projectID]; ok {
			projects[i].Tags = append(projects[i].Tags, tag)
		}
	}
	return rows.Err()
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

func (r *ProjectRepository) CreateWithMetadata(ctx context.Context, p *models.Project, tags *[]string, publisherRole *string, initiatingSchoolID *int, milestones *[]models.ProjectMilestone, members *[]models.ProjectMember, eventIDs *[]int) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
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
	result, err := tx.NamedExecContext(ctx, query, p)
	if err != nil {
		return fmt.Errorf("create project: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	p.ID = int(id)
	if err := saveProjectMetadataTx(ctx, tx, p.ID, tags, publisherRole, initiatingSchoolID, milestones, members, eventIDs); err != nil {
		return err
	}
	return tx.Commit()
}

// Update updates a project
func (r *ProjectRepository) Update(ctx context.Context, p *models.Project) error {
	query := `
		UPDATE project SET
			name                 = :name,
			description          = :description,
			school_id            = :school_id,
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

func (r *ProjectRepository) UpdateWithMetadata(ctx context.Context, p *models.Project, tags *[]string, publisherRole *string, initiatingSchoolID *int, milestones *[]models.ProjectMilestone, members *[]models.ProjectMember, eventIDs *[]int) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	query := `
		UPDATE project SET
			name                 = :name,
			description          = :description,
			school_id            = :school_id,
			direction            = :direction,
			member_count         = :member_count,
			is_cross_school      = :is_cross_school,
			education_requirement = :education_requirement,
			skill_requirement    = :skill_requirement,
			updated_at           = CURRENT_TIMESTAMP
		WHERE id = :id
	`
	result, err := tx.NamedExecContext(ctx, query, p)
	if err != nil {
		return fmt.Errorf("update project: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("project not found")
	}
	if err := saveProjectMetadataTx(ctx, tx, p.ID, tags, publisherRole, initiatingSchoolID, milestones, members, eventIDs); err != nil {
		return err
	}
	return tx.Commit()
}

func saveProjectMetadataTx(ctx context.Context, tx *sqlx.Tx, projectID int, tags *[]string, publisherRole *string, initiatingSchoolID *int, milestones *[]models.ProjectMilestone, members *[]models.ProjectMember, eventIDs *[]int) error {
	if publisherRole != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE project SET publisher_role=? WHERE id=?", *publisherRole, projectID); err != nil {
			return err
		}
	}
	if initiatingSchoolID != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE project SET initiating_school_id=? WHERE id=?", *initiatingSchoolID, projectID); err != nil {
			return err
		}
	}
	if tags != nil {
		if _, err := tx.ExecContext(ctx, "DELETE FROM project_tag_relation WHERE project_id=?", projectID); err != nil {
			return err
		}
		for _, name := range *tags {
			res, err := tx.ExecContext(ctx, `INSERT INTO project_tag(name,status) VALUES(?,1)
				ON DUPLICATE KEY UPDATE id=LAST_INSERT_ID(id),status=1`, name)
			if err != nil {
				return err
			}
			tagID, err := res.LastInsertId()
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO project_tag_relation(project_id,tag_id) VALUES(?,?)", projectID, tagID); err != nil {
				return err
			}
		}
	}
	if milestones != nil {
		if _, err := tx.ExecContext(ctx, "DELETE FROM project_milestones WHERE project_id=?", projectID); err != nil {
			return err
		}
		for i := range *milestones {
			m := (*milestones)[i]
			sortOrder := i + 1
			if _, err := tx.ExecContext(ctx, `INSERT INTO project_milestones(project_id,milestone_date,description,sort_order)
				VALUES(?,?,?,?)`, projectID, m.MilestoneDate, m.Description, sortOrder); err != nil {
				return err
			}
		}
	}
	if members != nil {
		if _, err := tx.ExecContext(ctx, "DELETE FROM project_members WHERE project_id=?", projectID); err != nil {
			return err
		}
		for _, member := range *members {
			if _, err := tx.ExecContext(ctx, `INSERT INTO project_members(project_id,user_id,role)
				VALUES(?,?,?)`, projectID, member.UserID, member.Role); err != nil {
				return err
			}
		}
	}
	if eventIDs != nil {
		if _, err := tx.ExecContext(ctx, "DELETE FROM project_event WHERE project_id=?", projectID); err != nil {
			return err
		}
		for _, eventID := range uniquePositiveIDs(*eventIDs) {
			if _, err := tx.ExecContext(ctx, `INSERT IGNORE INTO project_event(project_id,event_id) VALUES(?,?)`, projectID, eventID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *ProjectRepository) ListMilestones(ctx context.Context, projectID int) ([]models.ProjectMilestone, error) {
	var milestones []models.ProjectMilestone
	if err := r.db.SelectContext(ctx, &milestones, `SELECT id,project_id,milestone_date,description,sort_order
		FROM project_milestones WHERE project_id=? ORDER BY milestone_date ASC, id ASC`, projectID); err != nil {
		return nil, fmt.Errorf("query project milestones: %w", err)
	}
	return milestones, nil
}

func (r *ProjectRepository) ListMembers(ctx context.Context, projectID int) ([]models.ProjectMember, error) {
	var members []models.ProjectMember
	if err := r.db.SelectContext(ctx, &members, `SELECT pm.id,pm.project_id,pm.user_id,pm.role,pm.created_at,pr.name AS role_name
		FROM project_members pm
		LEFT JOIN project_role pr ON pr.code=pm.role
		WHERE pm.project_id=?
		ORDER BY FIELD(pm.role,'TEAM_LEADER','TECH_LEADER','PRODUCT_MANAGER','TEAM_MEMBER'), pm.id ASC`, projectID); err != nil {
		return nil, fmt.Errorf("query project members: %w", err)
	}
	if len(members) == 0 {
		return members, nil
	}
	userIDs := make([]int, 0, len(members))
	for i := range members {
		userIDs = append(userIDs, members[i].UserID)
	}
	query, args, err := sqlx.In(`SELECT u.id,u.openid,u.nickname,u.avatar_url,u.auth_status,s.school_name,m.major_name,tp.id talent_profile_id
		FROM `+"`user`"+` u
		LEFT JOIN school s ON s.id=u.school_id
		LEFT JOIN major m ON m.id=u.major_id
		LEFT JOIN talent_profile tp ON tp.user_id=u.id
		WHERE u.id IN (?)`, userIDs)
	if err != nil {
		return nil, err
	}
	var users []models.User
	if err := r.db.SelectContext(ctx, &users, r.db.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("query project member users: %w", err)
	}
	userMap := make(map[int]*models.User, len(users))
	for i := range users {
		user := users[i]
		userMap[user.ID] = &user
	}
	for i := range members {
		members[i].User = userMap[members[i].UserID]
	}
	return members, nil
}

func (r *ProjectRepository) AddMembers(ctx context.Context, projectID int, members []models.ProjectMember) error {
	if len(members) == 0 {
		return nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, member := range members {
		if _, err := tx.ExecContext(ctx, `INSERT INTO project_members(project_id,user_id,role)
			VALUES(?,?,?)
			ON DUPLICATE KEY UPDATE role=role`, projectID, member.UserID, member.Role); err != nil {
			return fmt.Errorf("add project member: %w", err)
		}
	}
	return tx.Commit()
}

func (r *ProjectRepository) ReplaceMembers(ctx context.Context, projectID int, members []models.ProjectMember) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "DELETE FROM project_members WHERE project_id=?", projectID); err != nil {
		return fmt.Errorf("delete project members: %w", err)
	}
	for _, member := range members {
		if _, err := tx.ExecContext(ctx, `INSERT INTO project_members(project_id,user_id,role)
			VALUES(?,?,?)`, projectID, member.UserID, member.Role); err != nil {
			return fmt.Errorf("replace project member: %w", err)
		}
	}
	return tx.Commit()
}

func (r *ProjectRepository) GetMemberRole(ctx context.Context, projectID, userID int) (*string, error) {
	var role string
	if err := r.db.QueryRowxContext(ctx, `SELECT role FROM project_members WHERE project_id=? AND user_id=?`, projectID, userID).Scan(&role); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query project member role: %w", err)
	}
	return &role, nil
}

func (r *ProjectRepository) IsOwnerOrMember(ctx context.Context, projectID, userID int) (bool, error) {
	var exists bool
	if err := r.db.QueryRowxContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM project WHERE id=? AND creator_id=?
		UNION
		SELECT 1 FROM project_members WHERE project_id=? AND user_id=?
	)`, projectID, userID, projectID, userID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check project owner or member: %w", err)
	}
	return exists, nil
}

// HasUnreadPassiveStatusChange reports whether the user's own/member projects have
// a passive review status change after they last visited the "my projects" page.
func (r *ProjectRepository) HasUnreadPassiveStatusChange(ctx context.Context, userID int) (bool, error) {
	var exists bool
	err := r.db.QueryRowxContext(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM project p
			LEFT JOIN `+"`user`"+` u ON u.id = ?
			WHERE
				(p.creator_id = ? OR EXISTS (
					SELECT 1 FROM project_members pm
					WHERE pm.project_id = p.id AND pm.user_id = ?
				))
				AND p.status IN (?, ?)
				AND p.passive_status_changed_at > COALESCE(u.last_viewed_my_projects_at, CAST('1970-01-01 00:00:01' AS DATETIME))
		)
	`, userID, userID, userID, models.ProjectStatusApproved, models.ProjectStatusRejected).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check unread passive project status change: %w", err)
	}
	return exists, nil
}

func (r *ProjectRepository) MarkPassiveStatusChange(ctx context.Context, id int) error {
	result, err := r.db.ExecContext(ctx, `UPDATE project SET passive_status_changed_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("mark passive project status change: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("project not found")
	}
	return nil
}

func (r *ProjectRepository) RoleExists(ctx context.Context, role string) (bool, error) {
	var exists bool
	if err := r.db.QueryRowxContext(ctx, `SELECT EXISTS(SELECT 1 FROM project_role WHERE code=? AND status=1)`, role).Scan(&exists); err != nil {
		return false, fmt.Errorf("check project role: %w", err)
	}
	return exists, nil
}

// Delete performs a logical delete (sets status to DELETING).
func (r *ProjectRepository) Delete(ctx context.Context, id int) error {
	query := `UPDATE project SET status = ?, deleted_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query, models.ProjectStatusDeleting, id)
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
	query := `UPDATE project
		SET status = ?,
			reject_reason = CASE WHEN ? = ? THEN reject_reason ELSE NULL END,
			deleted_at = CASE WHEN ? = ? THEN deleted_at ELSE NULL END,
			recruit_completed_at = CASE WHEN ? = ? THEN CURRENT_TIMESTAMP ELSE recruit_completed_at END,
			ended_at = CASE WHEN ? = ? THEN CURRENT_TIMESTAMP ELSE ended_at END,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query,
		status,
		status, models.ProjectStatusRejected,
		status, models.ProjectStatusDeleting,
		status, models.ProjectStatusRecruitCompleted,
		status, models.ProjectStatusEnded,
		id,
	)
	if err != nil {
		return fmt.Errorf("update project status: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("project not found")
	}
	return nil
}

func (r *ProjectRepository) UpdateStatusWithRejectReason(ctx context.Context, id int, status int, rejectReason *string) error {
	query := `UPDATE project SET status = ?, reject_reason = ?, deleted_at = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query, status, rejectReason, id)
	if err != nil {
		return fmt.Errorf("update project status with reject reason: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("project not found")
	}
	return nil
}

func (r *ProjectRepository) CompleteRecruit(ctx context.Context, id int) (int64, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx,
		`UPDATE project SET status = ?, reject_reason = NULL, deleted_at = NULL, recruit_completed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		models.ProjectStatusRecruitCompleted, id,
	)
	if err != nil {
		return 0, fmt.Errorf("complete recruit project status: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return 0, fmt.Errorf("project not found")
	}

	result, err = tx.ExecContext(ctx,
		`UPDATE project_application SET status = ?, rejected_at = COALESCE(rejected_at, CURRENT_TIMESTAMP), updated_at = CURRENT_TIMESTAMP WHERE project_id = ? AND status IN (?, ?)`,
		models.ApplicationStatusRejected, id, models.ApplicationStatusPending, models.ApplicationStatusDiscussing,
	)
	if err != nil {
		return 0, fmt.Errorf("reject pending applications after complete recruit: %w", err)
	}
	pendingRejected, _ := result.RowsAffected()

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return pendingRejected, nil
}

// IncrementViewCount increments the view count of a project
func (r *ProjectRepository) IncrementViewCount(ctx context.Context, id int) error {
	query := `UPDATE project SET view_count = view_count + 1 WHERE id = ?`
	if _, err := r.db.ExecContext(ctx, query, id); err != nil {
		return fmt.Errorf("increment view count: %w", err)
	}
	return nil
}
