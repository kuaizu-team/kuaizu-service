package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

// TalentProfileRepository handles talent profile database operations
type TalentProfileRepository struct {
	db *sqlx.DB
}

// NewTalentProfileRepository creates a new TalentProfileRepository
func NewTalentProfileRepository(db *sqlx.DB) *TalentProfileRepository {
	return &TalentProfileRepository{db: db}
}

// TalentProfileListParams contains parameters for listing talent profiles
type TalentProfileListParams struct {
	Page         int
	Size         int
	SchoolID     *int
	MajorID      *int
	Keyword      *string
	Status       *int
	SortBy       *string // "school_priority" enables multi-tier priority ordering
	UserSchoolID *int    // raw school ID from caller

	// Pre-fetched by handler before calling List — used to build ORDER BY tiers.
	UserSchoolProvince *string
	UserSchoolCity     *string
	UserSchoolDistrict *string
	UserMajorClassID   *int // class_id of the user's major
}

// enrichSchoolMajor 为单条 TalentProfile 分别查 school/major 并回填名称
func (r *TalentProfileRepository) enrichSchoolMajor(ctx context.Context, p *models.TalentProfile) error {
	if p.SchoolID != nil {
		var name string
		if err := r.db.QueryRowxContext(ctx,
			"SELECT school_name FROM school WHERE id = ?", *p.SchoolID,
		).Scan(&name); err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("query school name: %w", err)
		} else if err == nil {
			p.SchoolName = &name
		}
	}
	if p.MajorID != nil {
		var name string
		if err := r.db.QueryRowxContext(ctx,
			"SELECT major_name FROM major WHERE id = ?", *p.MajorID,
		).Scan(&name); err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("query major name: %w", err)
		} else if err == nil {
			p.MajorName = &name
		}
	}
	return nil
}

// queryNameByIDs 通用 IN 查询辅助：给定 query（含单个 ? 作为 IN 参数）和 id 集合，
// 返回 id -> name 的映射。集合为空时直接返回空 map。
// 使用 sqlx.In 展开占位符，db.Rebind 适配驱动方言。
func (r *TalentProfileRepository) queryNameByIDs(
	ctx context.Context,
	query string,
	ids map[int]struct{},
) (map[int]string, error) {
	result := map[int]string{}
	if len(ids) == 0 {
		return result, nil
	}
	args := make([]int, 0, len(ids))
	for id := range ids {
		args = append(args, id)
	}
	q, params, err := sqlx.In(query, args)
	if err != nil {
		return nil, err
	}
	q = r.db.Rebind(q)
	rows, err := r.db.QueryxContext(ctx, q, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		result[id] = name
	}
	return result, rows.Err()
}

// enrichSchoolMajorBatch 批量为多条 TalentProfile 回填 school_name / major_name
// 收集所有唯一 school_id / major_id，各做一次 IN 查询，在内存中聚合
func (r *TalentProfileRepository) enrichSchoolMajorBatch(ctx context.Context, profiles []models.TalentProfile) error {
	// Collect unique IDs
	schoolIDs := map[int]struct{}{}
	majorIDs := map[int]struct{}{}
	for _, p := range profiles {
		if p.SchoolID != nil {
			schoolIDs[*p.SchoolID] = struct{}{}
		}
		if p.MajorID != nil {
			majorIDs[*p.MajorID] = struct{}{}
		}
	}

	// Query school names
	schoolNames, err := r.queryNameByIDs(ctx, "SELECT id, school_name FROM school WHERE id IN (?)", schoolIDs)
	if err != nil {
		return fmt.Errorf("batch query school names: %w", err)
	}

	// Query major names
	majorNames, err := r.queryNameByIDs(ctx, "SELECT id, major_name FROM major WHERE id IN (?)", majorIDs)
	if err != nil {
		return fmt.Errorf("batch query major names: %w", err)
	}

	// Fill back
	for i := range profiles {
		if profiles[i].SchoolID != nil {
			if name, ok := schoolNames[*profiles[i].SchoolID]; ok {
				profiles[i].SchoolName = &name
			}
		}
		if profiles[i].MajorID != nil {
			if name, ok := majorNames[*profiles[i].MajorID]; ok {
				profiles[i].MajorName = &name
			}
		}
	}
	return nil
}

// List retrieves paginated talent profiles with optional filters and multi-tier smart sorting.
//
// Sorting scenarios (activated by SortBy == "school_priority"):
//
//	Scenario A — UserSchoolID + UserMajorClassID both set:
//	  10 tiers: [same school + same class] → [same school] → [same district + same class] →
//	            [same district] → [same city + same class] → [same city] →
//	            [same province + same class] → [same province] → [same class] → [other]
//	Scenario B — UserSchoolID only:
//	  5 tiers: [same school] → [same district] → [same city] → [same province] → [other]
//	Scenario C — UserMajorClassID only:
//	  2 tiers: [same class] → [other]
//	Scenario D — neither set (or SortBy != "school_priority"):
//	  plain tp.updated_at DESC
//
// Within every tier the tiebreak is tp.updated_at DESC.
// Geo comparisons use the talent's school columns (ts.*) from a conditional LEFT JOIN.
// Major class comparisons use the talent's major columns (tm.*) from a conditional LEFT JOIN.
func (r *TalentProfileRepository) List(ctx context.Context, params TalentProfileListParams) ([]models.TalentProfile, int64, error) {
	// ── WHERE clause ────────────────────────────────────────────────────────────
	conditions := []string{"tp.status = 1"}
	whereArgs := []interface{}{}

	if params.SchoolID != nil {
		conditions = append(conditions, "u.school_id = ?")
		whereArgs = append(whereArgs, *params.SchoolID)
	}
	if params.MajorID != nil {
		conditions = append(conditions, "u.major_id = ?")
		whereArgs = append(whereArgs, *params.MajorID)
	}
	if params.Keyword != nil && *params.Keyword != "" {
		conditions = append(conditions, "(u.nickname LIKE ? OR tp.self_evaluation LIKE ? OR tp.skill_summary LIKE ?)")
		pattern := "%" + *params.Keyword + "%"
		whereArgs = append(whereArgs, pattern, pattern, pattern)
	}
	if params.Status != nil {
		conditions = append(conditions, "tp.status = ?")
		whereArgs = append(whereArgs, *params.Status)
	}

	whereClause := strings.Join(conditions, " AND ")

	// ── COUNT (WHERE args only; no ORDER BY args needed) ────────────────────────
	countQuery := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM talent_profile tp
		LEFT JOIN `+"`user`"+` u ON tp.user_id = u.id
		WHERE %s
	`, whereClause)
	var total int64
	if err := r.db.QueryRowxContext(ctx, countQuery, whereArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count talent profiles: %w", err)
	}

	// ── ORDER BY (multi-tier CASE WHEN) ─────────────────────────────────────────
	orderClause := "tp.updated_at DESC"
	var orderArgs []interface{}

	// Determine available geo / major data
	schoolID := 0
	if params.UserSchoolID != nil {
		schoolID = *params.UserSchoolID
	}
	classID := 0
	if params.UserMajorClassID != nil {
		classID = *params.UserMajorClassID
	}
	hasSchool := schoolID != 0
	hasMajor := classID != 0

	district, city, province := "", "", ""
	if params.UserSchoolDistrict != nil {
		district = *params.UserSchoolDistrict
	}
	if params.UserSchoolCity != nil {
		city = *params.UserSchoolCity
	}
	if params.UserSchoolProvince != nil {
		province = *params.UserSchoolProvince
	}
	hasDistrict := hasSchool && district != "" && city != ""
	hasCity := hasSchool && city != ""
	hasProvince := hasSchool && province != ""

	needsSchoolJoin := false // LEFT JOIN school ts ON u.school_id = ts.id
	needsMajorJoin := false  // LEFT JOIN major  tm ON u.major_id  = tm.id

	if params.SortBy != nil && *params.SortBy == "school_priority" && (hasSchool || hasMajor) {
		var whenClauses []string

		switch {
		// ── Scenario A: school + major (10-tier) ────────────────────────────
		case hasSchool && hasMajor:
			needsSchoolJoin = hasDistrict || hasCity || hasProvince // ts.* only referenced when geo tiers exist
			needsMajorJoin = true

			whenClauses = append(whenClauses, "WHEN u.school_id = ? AND tm.class_id = ? THEN 1")
			orderArgs = append(orderArgs, schoolID, classID)

			whenClauses = append(whenClauses, "WHEN u.school_id = ? THEN 2")
			orderArgs = append(orderArgs, schoolID)

			if hasDistrict {
				whenClauses = append(whenClauses, "WHEN ts.district = ? AND ts.city = ? AND tm.class_id = ? THEN 3")
				orderArgs = append(orderArgs, district, city, classID)

				whenClauses = append(whenClauses, "WHEN ts.district = ? AND ts.city = ? THEN 4")
				orderArgs = append(orderArgs, district, city)
			}
			if hasCity {
				whenClauses = append(whenClauses, "WHEN ts.city = ? AND tm.class_id = ? THEN 5")
				orderArgs = append(orderArgs, city, classID)

				whenClauses = append(whenClauses, "WHEN ts.city = ? THEN 6")
				orderArgs = append(orderArgs, city)
			}
			if hasProvince {
				whenClauses = append(whenClauses, "WHEN ts.province = ? AND tm.class_id = ? THEN 7")
				orderArgs = append(orderArgs, province, classID)

				whenClauses = append(whenClauses, "WHEN ts.province = ? THEN 8")
				orderArgs = append(orderArgs, province)
			}

			whenClauses = append(whenClauses, "WHEN tm.class_id = ? THEN 9")
			orderArgs = append(orderArgs, classID)

			tierExpr := "CASE\n" + strings.Join(whenClauses, "\n") + "\nELSE 10\nEND"
			orderClause = tierExpr + " ASC, tp.updated_at DESC"

		// ── Scenario B: school only (5-tier) ────────────────────────────────
		case hasSchool:
			needsSchoolJoin = hasDistrict || hasCity || hasProvince // ts.* only referenced when geo tiers exist

			whenClauses = append(whenClauses, "WHEN u.school_id = ? THEN 1")
			orderArgs = append(orderArgs, schoolID)

			if hasDistrict {
				whenClauses = append(whenClauses, "WHEN ts.district = ? AND ts.city = ? THEN 2")
				orderArgs = append(orderArgs, district, city)
			}
			if hasCity {
				whenClauses = append(whenClauses, "WHEN ts.city = ? THEN 3")
				orderArgs = append(orderArgs, city)
			}
			if hasProvince {
				whenClauses = append(whenClauses, "WHEN ts.province = ? THEN 4")
				orderArgs = append(orderArgs, province)
			}

			tierExpr := "CASE\n" + strings.Join(whenClauses, "\n") + "\nELSE 5\nEND"
			orderClause = tierExpr + " ASC, tp.updated_at DESC"

		// ── Scenario C: major only (2-tier) ─────────────────────────────────
		case hasMajor:
			needsMajorJoin = true

			whenClauses = append(whenClauses, "WHEN tm.class_id = ? THEN 1")
			orderArgs = append(orderArgs, classID)

			tierExpr := "CASE\n" + strings.Join(whenClauses, "\n") + "\nELSE 2\nEND"
			orderClause = tierExpr + " ASC, tp.updated_at DESC"
		}
		// Scenario D: neither hasSchool nor hasMajor → keep default "tp.updated_at DESC"
	}

	// ── Prepend auth_status sort key for all school_priority requests ────────────
	// Certified users (auth_status = 1) surface before uncertified ones;
	// within each auth group the existing tier / updated_at order is preserved.
	// This runs after the switch so it wraps all four scenarios uniformly.
	if params.SortBy != nil && *params.SortBy == "school_priority" {
		const authExpr = "CASE WHEN u.auth_status = 1 THEN 0 ELSE 1 END"
		orderClause = authExpr + " ASC, " + orderClause
	}

	// ── Build optional extra JOINs ───────────────────────────────────────────────
	extraJoins := ""
	if needsSchoolJoin {
		extraJoins += "\n\t\tLEFT JOIN school ts ON u.school_id = ts.id"
	}
	if needsMajorJoin {
		extraJoins += "\n\t\tLEFT JOIN major tm ON u.major_id = tm.id"
	}

	// ── Main data query ─────────────────────────────────────────────────────────
	offset := (params.Page - 1) * params.Size
	query := fmt.Sprintf(`
		SELECT
			tp.id, tp.user_id, tp.self_evaluation, tp.skill_summary,
			tp.project_experience, tp.mbti, tp.status, tp.reject_reason, tp.view_count,
			tp.created_at, tp.updated_at,
			u.nickname, u.phone, u.email, u.avatar_url,
			u.school_id, u.major_id, u.grade, u.auth_status
		FROM talent_profile tp
		LEFT JOIN `+"`user`"+` u ON tp.user_id = u.id%s
		WHERE %s
		ORDER BY %s
		LIMIT ? OFFSET ?
	`, extraJoins, whereClause, orderClause)

	// Combine: WHERE args → ORDER BY args → LIMIT/OFFSET
	dataArgs := make([]interface{}, 0, len(whereArgs)+len(orderArgs)+2)
	dataArgs = append(dataArgs, whereArgs...)
	dataArgs = append(dataArgs, orderArgs...)
	dataArgs = append(dataArgs, params.Size, offset)

	var profiles []models.TalentProfile
	if err := r.db.SelectContext(ctx, &profiles, query, dataArgs...); err != nil {
		log.Printf("query talent profiles: %v", err)
		return nil, 0, fmt.Errorf("query talent profiles: %w", err)
	}

	// Enrich school_name / major_name via batch follow-up queries (single IN query each)
	if err := r.enrichSchoolMajorBatch(ctx, profiles); err != nil {
		log.Printf("enrich school major batch: %v", err)
		return nil, 0, err
	}

	return profiles, total, nil
}

// GetByID retrieves a talent profile by ID with user info
func (r *TalentProfileRepository) GetByID(ctx context.Context, id int) (*models.TalentProfile, error) {
	// talent_profile + user (2 tables)
	query := `
		SELECT 
			tp.id, tp.user_id, tp.self_evaluation, tp.skill_summary,
			tp.project_experience, tp.mbti, tp.status, tp.reject_reason, tp.view_count,
			tp.created_at, tp.updated_at,
			u.nickname, u.phone, u.email, u.wechat_id, u.avatar_url,
			u.school_id, u.major_id, u.grade, u.auth_status
		FROM talent_profile tp
		LEFT JOIN ` + "`user`" + ` u ON tp.user_id = u.id
		WHERE tp.id = ?
	`

	var p models.TalentProfile
	if err := r.db.QueryRowxContext(ctx, query, id).StructScan(&p); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query talent profile by id: %w", err)
	}

	// Follow-up: school and major (single-table each)
	if err := r.enrichSchoolMajor(ctx, &p); err != nil {
		return nil, err
	}

	return &p, nil
}

// GetByUserID retrieves a talent profile by user ID
func (r *TalentProfileRepository) GetByUserID(ctx context.Context, userID int) (*models.TalentProfile, error) {
	// talent_profile + user (2 tables)
	query := `
		SELECT 
			tp.id, tp.user_id, tp.self_evaluation, tp.skill_summary,
			tp.project_experience, tp.mbti, tp.status, tp.reject_reason, tp.view_count,
			tp.created_at, tp.updated_at,
			u.nickname, u.phone, u.email, u.wechat_id, u.avatar_url,
			u.school_id, u.major_id, u.grade, u.auth_status
		FROM talent_profile tp
		LEFT JOIN ` + "`user`" + ` u ON tp.user_id = u.id
		WHERE tp.user_id = ?
	`

	var p models.TalentProfile
	if err := r.db.QueryRowxContext(ctx, query, userID).StructScan(&p); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		log.Printf("query talent profile by user id: %v", err)
		return nil, fmt.Errorf("query talent profile by user id: %w", err)
	}

	// Follow-up: school and major (single-table each)
	if err := r.enrichSchoolMajor(ctx, &p); err != nil {
		log.Printf("enrich school major: %v", err)
		return nil, err
	}

	return &p, nil
}

// Upsert creates or updates a talent profile for a user
func (r *TalentProfileRepository) Upsert(ctx context.Context, p *models.TalentProfile) error {
	// Check if profile exists
	existing, err := r.GetByUserID(ctx, p.UserID)
	if err != nil {
		return err
	}

	if existing == nil {
		// Insert
		query := `
			INSERT INTO talent_profile (
				user_id, self_evaluation, skill_summary, project_experience,
				mbti, status
			) VALUES (
				:user_id, :self_evaluation, :skill_summary, :project_experience,
				:mbti, :status
			)
		`
		result, err := r.db.NamedExecContext(ctx, query, p)
		if err != nil {
			return fmt.Errorf("insert talent profile: %w", err)
		}
		id, _ := result.LastInsertId()
		p.ID = int(id)
	} else {
		// Update
		query := `
			UPDATE talent_profile SET
				self_evaluation = :self_evaluation,
				skill_summary = :skill_summary,
				project_experience = :project_experience,
				mbti = :mbti,
				status = :status,
				reject_reason = CASE WHEN :status = 2 THEN NULL ELSE reject_reason END,
				updated_at = CURRENT_TIMESTAMP
			WHERE user_id = :user_id
		`
		_, err := r.db.NamedExecContext(ctx, query, p)
		if err != nil {
			return fmt.Errorf("update talent profile: %w", err)
		}
		p.ID = existing.ID
	}

	return nil
}

// UpdateStatus updates the status of a talent profile by ID.
func (r *TalentProfileRepository) UpdateStatus(ctx context.Context, id int, status int, rejectReason *string) error {
	query := `
		UPDATE talent_profile
		SET status = ?, reject_reason = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	_, err := r.db.ExecContext(ctx, query, status, rejectReason, id)
	if err != nil {
		return fmt.Errorf("update talent profile status: %w", err)
	}
	return nil
}

// DeleteByUserID deletes a talent profile by user ID
func (r *TalentProfileRepository) DeleteByUserID(ctx context.Context, userID int) error {
	query := `
		UPDATE talent_profile SET status = 0 WHERE user_id = ?
	`
	_, err := r.db.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("delete talent profile by user id: %w", err)
	}
	return nil
}

// IsOwner checks if a user owns a talent profile.
func (r *TalentProfileRepository) IsOwner(ctx context.Context, talentID, userID int) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM talent_profile WHERE id = ? AND user_id = ?)`
	if err := r.db.QueryRowxContext(ctx, query, talentID, userID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check talent profile owner: %w", err)
	}
	return exists, nil
}

// IncrementViewCount increments the denormalized view count of a talent profile.
func (r *TalentProfileRepository) IncrementViewCount(ctx context.Context, id int) error {
	query := `UPDATE talent_profile SET view_count = view_count + 1 WHERE id = ?`
	if _, err := r.db.ExecContext(ctx, query, id); err != nil {
		return fmt.Errorf("increment talent profile view count: %w", err)
	}
	return nil
}
