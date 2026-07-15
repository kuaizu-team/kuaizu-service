package repository

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

// AdminUserRepository handles admin user database operations
type AdminUserRepository struct {
	db *sqlx.DB
}

// NewAdminUserRepository creates a new AdminUserRepository
func NewAdminUserRepository(db *sqlx.DB) *AdminUserRepository {
	return &AdminUserRepository{db: db}
}

// adminUserCols is the common SELECT column list (requires LEFT JOIN school s ON au.school_id = s.id)
const adminUserCols = `
	au.id, au.username, au.password_hash, au.password_encrypted, au.nickname,
	au.role, au.school_id, au.status, au.finance_remark, au.commission_rate, au.join_date, au.intro, au.article_url, au.created_at, au.updated_at,
	s.school_name`

const adminUserFrom = `
	FROM admin_user au
	LEFT JOIN school s ON au.school_id = s.id`

// GetByUsername retrieves an admin user by username (includes role, school_id, school_name)
func (r *AdminUserRepository) GetByUsername(ctx context.Context, username string) (*models.AdminUser, error) {
	query := `SELECT ` + adminUserCols + adminUserFrom + ` WHERE au.username = ?`

	var admin models.AdminUser
	if err := r.db.QueryRowxContext(ctx, query, username).StructScan(&admin); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query admin by username: %w", err)
	}

	if err := r.loadSchoolRelations(ctx, []*models.AdminUser{&admin}); err != nil {
		return nil, err
	}
	return &admin, nil
}

// GetByID retrieves an admin user by ID (includes role, school_id, school_name)
func (r *AdminUserRepository) GetByID(ctx context.Context, id int) (*models.AdminUser, error) {
	query := `SELECT ` + adminUserCols + adminUserFrom + ` WHERE au.id = ?`

	var admin models.AdminUser
	if err := r.db.QueryRowxContext(ctx, query, id).StructScan(&admin); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query admin by id: %w", err)
	}

	if err := r.loadSchoolRelations(ctx, []*models.AdminUser{&admin}); err != nil {
		return nil, err
	}
	return &admin, nil
}

// AdminUserListParams contains parameters for listing admin users
type AdminUserListParams struct {
	Page                    int
	Size                    int
	Keyword                 *string // 按 username/nickname 模糊搜索
	Role                    *int    // 按角色筛选（1/2/3）
	Status                  *int    // 按状态筛选（0/1）
	SchoolID                *int    // 校区管理员只能看本校，超级管理员不传
	IncludeAllEventManagers bool    // legacy compatibility; normally false to preserve school isolation
	SchoolIDs               []int   // all schools owned by a school super admin
	ViewerAdminID           *int    // include the current school super admin in scoped lists
}

// List retrieves paginated admin users with optional filters
func (r *AdminUserRepository) List(ctx context.Context, params AdminUserListParams) ([]*models.AdminUser, int64, error) {
	conditions := []string{"1=1"}
	args := []interface{}{}

	if params.Keyword != nil && *params.Keyword != "" {
		conditions = append(conditions, "(au.username LIKE ? OR au.nickname LIKE ?)")
		args = append(args, "%"+*params.Keyword+"%", "%"+*params.Keyword+"%")
	}
	if params.Role != nil {
		conditions = append(conditions, "au.role = ?")
		args = append(args, *params.Role)
	}
	if params.Status != nil {
		conditions = append(conditions, "au.status = ?")
		args = append(args, *params.Status)
	}
	if params.SchoolID != nil {
		if params.IncludeAllEventManagers {
			conditions = append(conditions, "(au.school_id = ? OR au.role = ?)")
			args = append(args, *params.SchoolID, models.AdminRoleEventManager)
		} else {
			conditions = append(conditions, "au.school_id = ?")
			args = append(args, *params.SchoolID)
		}
	}
	if len(params.SchoolIDs) > 0 {
		viewerID := 0
		if params.ViewerAdminID != nil {
			viewerID = *params.ViewerAdminID
		}
		condition, inArgs, err := sqlx.In(`(
			au.id = ? OR (au.role IN (?, ?) AND au.school_id IN (?))
		)`, viewerID, models.AdminRoleSchoolAdmin, models.AdminRoleEventManager, params.SchoolIDs)
		if err != nil {
			return nil, 0, fmt.Errorf("build admin school scope: %w", err)
		}
		conditions = append(conditions, condition)
		args = append(args, inArgs...)
	}

	whereClause := strings.Join(conditions, " AND ")

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM admin_user au WHERE %s", whereClause)
	var total int64
	if err := r.db.QueryRowxContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count admins: %w", err)
	}

	offset := (params.Page - 1) * params.Size
	query := fmt.Sprintf(`SELECT `+adminUserCols+adminUserFrom+`
		WHERE %s
		ORDER BY au.created_at DESC
		LIMIT ? OFFSET ?`, whereClause)

	dataArgs := make([]interface{}, 0, len(args)+2)
	dataArgs = append(dataArgs, args...)
	dataArgs = append(dataArgs, params.Size, offset)
	var admins []*models.AdminUser
	if err := r.db.SelectContext(ctx, &admins, query, dataArgs...); err != nil {
		return nil, 0, fmt.Errorf("query admins: %w", err)
	}
	if err := r.loadSchoolRelations(ctx, admins); err != nil {
		return nil, 0, err
	}

	return admins, total, nil
}

func (r *AdminUserRepository) loadSchoolRelations(ctx context.Context, admins []*models.AdminUser) error {
	if len(admins) == 0 {
		return nil
	}
	ids := make([]int, 0, len(admins))
	byID := make(map[int]*models.AdminUser, len(admins))
	for _, admin := range admins {
		if admin == nil {
			continue
		}
		ids = append(ids, admin.ID)
		byID[admin.ID] = admin
		admin.Schools = []models.AdminSchoolRelation{}
	}
	if len(ids) == 0 {
		return nil
	}
	query, args, err := sqlx.In(`
		SELECT rel.id, rel.admin_user_id, rel.school_id, s.school_name,
		       rel.commission_rate, rel.is_owner, rel.created_at, rel.updated_at
		FROM admin_school_relation rel
		JOIN school s ON s.id = rel.school_id
		WHERE rel.admin_user_id IN (?)
		ORDER BY rel.is_owner DESC, rel.created_at ASC, rel.id ASC`, ids)
	if err != nil {
		return fmt.Errorf("build admin school relation query: %w", err)
	}
	var relations []models.AdminSchoolRelation
	if err := r.db.SelectContext(ctx, &relations, r.db.Rebind(query), args...); err != nil {
		return fmt.Errorf("query admin school relations: %w", err)
	}
	for _, relation := range relations {
		if admin := byID[relation.AdminUserID]; admin != nil {
			admin.Schools = append(admin.Schools, relation)
		}
	}
	return nil
}

// ListSchoolRelations returns settlement relations, optionally restricted to
// operational ownership.
func (r *AdminUserRepository) ListSchoolRelations(ctx context.Context, adminID int, ownersOnly bool) ([]models.AdminSchoolRelation, error) {
	query := `SELECT rel.id, rel.admin_user_id, rel.school_id, s.school_name,
	                 rel.commission_rate, rel.is_owner, rel.created_at, rel.updated_at
	          FROM admin_school_relation rel
	          JOIN school s ON s.id = rel.school_id
	          WHERE rel.admin_user_id = ?`
	if ownersOnly {
		query += " AND rel.is_owner = 1"
	}
	query += " ORDER BY rel.created_at ASC, rel.id ASC"
	var relations []models.AdminSchoolRelation
	if err := r.db.SelectContext(ctx, &relations, query, adminID); err != nil {
		return nil, fmt.Errorf("query admin school relations: %w", err)
	}
	return relations, nil
}

func (r *AdminUserRepository) AccessibleSchoolIDs(ctx context.Context, adminID int) ([]int, error) {
	var ids []int
	if err := r.db.SelectContext(ctx, &ids, `
		SELECT school_id FROM admin_school_relation
		WHERE admin_user_id = ? AND commission_rate > 0
		ORDER BY id`, adminID); err != nil {
		return nil, fmt.Errorf("query accessible school ids: %w", err)
	}
	if ids == nil {
		ids = []int{}
	}
	return ids, nil
}

func (r *AdminUserRepository) RemoveSchoolRelation(ctx context.Context, adminID, schoolID int) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM admin_school_relation WHERE admin_user_id=? AND school_id=?`, adminID, schoolID)
	if err != nil {
		return fmt.Errorf("remove admin school relation: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *AdminUserRepository) HasOwnedSchool(ctx context.Context, adminID, schoolID int) (bool, error) {
	var exists bool
	if err := r.db.QueryRowxContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM admin_school_relation
		WHERE admin_user_id = ? AND school_id = ? AND is_owner = 1
	)`, adminID, schoolID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check owned school: %w", err)
	}
	return exists, nil
}

// ErrDuplicateUsername is returned when a username conflicts with an existing record
var ErrDuplicateUsername = fmt.Errorf("账号已存在")

var (
	ErrSchoolAlreadyOwned      = fmt.Errorf("school already has an operational owner")
	ErrSchoolNotOwned          = fmt.Errorf("school is not owned by this admin")
	ErrCommissionRateExceeded  = fmt.Errorf("delegated commission exceeds source rate")
	ErrInvalidDelegationTarget = fmt.Errorf("invalid delegation target")
)

func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate entry") || strings.Contains(message, "duplicate key")
}

// CreateWithSchools creates an administrator and its owned school relations in
// one transaction. It is the authoritative create path for role=2.
func (r *AdminUserRepository) CreateWithSchools(ctx context.Context, admin *models.AdminUser, schools []models.AdminSchoolRelation) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin create admin transaction: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		INSERT INTO admin_user (username, password_hash, password_encrypted, nickname, role, school_id, status)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, admin.Username, admin.PasswordHash, admin.PasswordEncrypted,
		admin.Nickname, admin.Role, admin.SchoolID, admin.Status)
	if err != nil {
		if isDuplicateKeyError(err) {
			return ErrDuplicateUsername
		}
		return fmt.Errorf("create admin: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("read created admin id: %w", err)
	}
	admin.ID = int(id)
	for _, school := range schools {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO admin_school_relation (admin_user_id, school_id, commission_rate, is_owner)
			VALUES (?, ?, ?, 1)`, admin.ID, school.SchoolID, school.CommissionRate); err != nil {
			if isDuplicateKeyError(err) {
				return ErrSchoolAlreadyOwned
			}
			return fmt.Errorf("create admin school relation: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit create admin transaction: %w", err)
	}
	admin.Schools = schools
	return nil
}

func replaceOwnedSchoolsTx(ctx context.Context, tx *sqlx.Tx, adminID int, schools []models.AdminSchoolRelation) error {
	desired := make(map[int]models.AdminSchoolRelation, len(schools))
	for _, school := range schools {
		desired[school.SchoolID] = school
	}
	var current []int
	if err := tx.SelectContext(ctx, &current, `
		SELECT school_id FROM admin_school_relation
		WHERE admin_user_id = ? AND is_owner = 1 FOR UPDATE`, adminID); err != nil {
		return fmt.Errorf("lock current admin schools: %w", err)
	}
	for _, schoolID := range current {
		if _, keep := desired[schoolID]; !keep {
			if _, err := tx.ExecContext(ctx, `DELETE FROM admin_school_relation
				WHERE admin_user_id = ? AND school_id = ? AND is_owner = 1`, adminID, schoolID); err != nil {
				return fmt.Errorf("remove admin school relation: %w", err)
			}
		}
	}
	for _, school := range schools {
		result, err := tx.ExecContext(ctx, `UPDATE admin_school_relation
			SET commission_rate=?, is_owner=1, updated_at=CURRENT_TIMESTAMP
			WHERE admin_user_id=? AND school_id=?`, school.CommissionRate, adminID, school.SchoolID)
		if err == nil {
			if rows, _ := result.RowsAffected(); rows == 0 {
				var exists bool
				err = tx.QueryRowxContext(ctx, `SELECT EXISTS(SELECT 1 FROM admin_school_relation WHERE admin_user_id=? AND school_id=?)`, adminID, school.SchoolID).Scan(&exists)
				if err == nil && !exists {
					_, err = tx.ExecContext(ctx, `INSERT INTO admin_school_relation
						(admin_user_id,school_id,commission_rate,is_owner) VALUES(?,?,?,1)`,
						adminID, school.SchoolID, school.CommissionRate)
				}
			}
		}
		if err != nil {
			if isDuplicateKeyError(err) {
				return ErrSchoolAlreadyOwned
			}
			return fmt.Errorf("upsert admin school relation: %w", err)
		}
	}
	return nil
}

// UpdateWithSchools updates administrator fields and replaces only owned
// schools. Non-owner settlement relations are intentionally preserved.
func (r *AdminUserRepository) UpdateWithSchools(ctx context.Context, admin *models.AdminUser, schools []models.AdminSchoolRelation) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin update admin transaction: %w", err)
	}
	defer tx.Rollback()
	query := `UPDATE admin_user SET nickname=?, role=?, school_id=?, status=?, join_date=?, intro=?, article_url=?, updated_at=CURRENT_TIMESTAMP`
	args := []interface{}{admin.Nickname, admin.Role, admin.SchoolID, admin.Status, admin.JoinDate, admin.Intro, admin.ArticleURL}
	if admin.PasswordHash != "" {
		query += ", password_hash=?, password_encrypted=?"
		args = append(args, admin.PasswordHash, admin.PasswordEncrypted)
	}
	query += " WHERE id=?"
	args = append(args, admin.ID)
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update admin: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	if admin.Role == models.AdminRoleSchoolSuperAdmin {
		if err := replaceOwnedSchoolsTx(ctx, tx, admin.ID, schools); err != nil {
			return err
		}
	} else if _, err := tx.ExecContext(ctx, `DELETE FROM admin_school_relation WHERE admin_user_id=? AND is_owner=1`, admin.ID); err != nil {
		return fmt.Errorf("remove obsolete owned schools: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit update admin transaction: %w", err)
	}
	return nil
}

func (r *AdminUserRepository) UpdateSchoolCommission(ctx context.Context, adminID, schoolID int, rate float64) error {
	result, err := r.db.ExecContext(ctx, `UPDATE admin_school_relation
		SET commission_rate=?, updated_at=CURRENT_TIMESTAMP
		WHERE admin_user_id=? AND school_id=?`, rate, adminID, schoolID)
	if err != nil {
		return fmt.Errorf("update school commission rate: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *AdminUserRepository) SchoolCommissionTotalExcluding(ctx context.Context, schoolID, adminID int) (float64, error) {
	var total float64
	if err := r.db.QueryRowxContext(ctx, `SELECT COALESCE(SUM(commission_rate),0)
		FROM admin_school_relation WHERE school_id=? AND admin_user_id<>?`, schoolID, adminID).Scan(&total); err != nil {
		return 0, fmt.Errorf("query school commission total: %w", err)
	}
	return total, nil
}

// DelegateSchool atomically transfers operational ownership and splits the
// source administrator's settlement percentage.
func (r *AdminUserRepository) DelegateSchool(ctx context.Context, sourceAdminID int, target *models.AdminUser, targetUserID *int, schoolID int, rate float64) (int, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin delegation transaction: %w", err)
	}
	defer tx.Rollback()

	var sourceRate float64
	if err := tx.QueryRowxContext(ctx, `SELECT commission_rate FROM admin_school_relation
		WHERE admin_user_id=? AND school_id=? AND is_owner=1 FOR UPDATE`, sourceAdminID, schoolID).Scan(&sourceRate); err != nil {
		if err == sql.ErrNoRows {
			return 0, ErrSchoolNotOwned
		}
		return 0, fmt.Errorf("lock source school relation: %w", err)
	}
	if rate <= 0 || rate > sourceRate {
		return 0, ErrCommissionRateExceeded
	}

	resolvedTargetID := 0
	if targetUserID != nil {
		resolvedTargetID = *targetUserID
		var role, status int
		if err := tx.QueryRowxContext(ctx, `SELECT role,status FROM admin_user WHERE id=? FOR UPDATE`, resolvedTargetID).Scan(&role, &status); err != nil || role != models.AdminRoleSchoolSuperAdmin || status != models.AdminUserStatusEnabled || resolvedTargetID == sourceAdminID {
			return 0, ErrInvalidDelegationTarget
		}
	} else {
		if target == nil {
			return 0, ErrInvalidDelegationTarget
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO admin_user
			(username,password_hash,password_encrypted,nickname,role,school_id,status)
			VALUES(?,?,?,?,?,NULL,1)`, target.Username, target.PasswordHash, target.PasswordEncrypted,
			target.Nickname, models.AdminRoleSchoolSuperAdmin)
		if err != nil {
			if isDuplicateKeyError(err) {
				return 0, ErrDuplicateUsername
			}
			return 0, fmt.Errorf("create delegated admin: %w", err)
		}
		id, err := result.LastInsertId()
		if err != nil {
			return 0, fmt.Errorf("read delegated admin id: %w", err)
		}
		resolvedTargetID = int(id)
	}

	remaining := math.Round((sourceRate-rate)*100) / 100
	if remaining <= 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM admin_school_relation
			WHERE admin_user_id=? AND school_id=?`, sourceAdminID, schoolID); err != nil {
			return 0, fmt.Errorf("remove delegated source relation: %w", err)
		}
	} else if _, err := tx.ExecContext(ctx, `UPDATE admin_school_relation
		SET commission_rate=?, is_owner=0, updated_at=CURRENT_TIMESTAMP
		WHERE admin_user_id=? AND school_id=?`, remaining, sourceAdminID, schoolID); err != nil {
		return 0, fmt.Errorf("downgrade delegated source relation: %w", err)
	}

	result, err := tx.ExecContext(ctx, `UPDATE admin_school_relation
		SET commission_rate=?, is_owner=1, updated_at=CURRENT_TIMESTAMP
		WHERE admin_user_id=? AND school_id=?`, rate, resolvedTargetID, schoolID)
	if err == nil {
		if rows, _ := result.RowsAffected(); rows == 0 {
			var exists bool
			err = tx.QueryRowxContext(ctx, `SELECT EXISTS(SELECT 1 FROM admin_school_relation WHERE admin_user_id=? AND school_id=?)`, resolvedTargetID, schoolID).Scan(&exists)
			if err == nil && !exists {
				_, err = tx.ExecContext(ctx, `INSERT INTO admin_school_relation
					(admin_user_id,school_id,commission_rate,is_owner) VALUES(?,?,?,1)`, resolvedTargetID, schoolID, rate)
			}
		}
	}
	if err != nil {
		if isDuplicateKeyError(err) {
			return 0, ErrSchoolAlreadyOwned
		}
		return 0, fmt.Errorf("create delegated target relation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit delegation transaction: %w", err)
	}
	return resolvedTargetID, nil
}

// Create inserts a new admin user and populates its ID
func (r *AdminUserRepository) Create(ctx context.Context, admin *models.AdminUser) error {
	query := `
		INSERT INTO admin_user (username, password_hash, password_encrypted, nickname, role, school_id, status)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	result, err := r.db.ExecContext(ctx, query,
		admin.Username, admin.PasswordHash, admin.PasswordEncrypted, admin.Nickname,
		admin.Role, admin.SchoolID, admin.Status)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") || strings.Contains(err.Error(), "duplicate key") {
			return ErrDuplicateUsername
		}
		return fmt.Errorf("create admin: %w", err)
	}
	id, _ := result.LastInsertId()
	admin.ID = int(id)
	return nil
}

// Update updates all mutable fields of an admin user.
// When PasswordHash is empty the password column is left untouched.
func (r *AdminUserRepository) Update(ctx context.Context, admin *models.AdminUser) error {
	var (
		query string
		args  []interface{}
	)
	if admin.PasswordHash != "" {
		query = `UPDATE admin_user
			SET nickname = ?, role = ?, school_id = ?, status = ?, join_date = ?, intro = ?, article_url = ?, password_hash = ?, password_encrypted = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?`
		args = []interface{}{admin.Nickname, admin.Role, admin.SchoolID, admin.Status, admin.JoinDate, admin.Intro, admin.ArticleURL, admin.PasswordHash, admin.PasswordEncrypted, admin.ID}
	} else {
		query = `UPDATE admin_user
			SET nickname = ?, role = ?, school_id = ?, status = ?, join_date = ?, intro = ?, article_url = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?`
		args = []interface{}{admin.Nickname, admin.Role, admin.SchoolID, admin.Status, admin.JoinDate, admin.Intro, admin.ArticleURL, admin.ID}
	}

	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update admin: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpdateStatus updates only the status field
func (r *AdminUserRepository) UpdateStatus(ctx context.Context, id int, status int) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE admin_user SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, status, id)
	if err != nil {
		return fmt.Errorf("update admin status: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpdateFinanceRemark updates only the finance remark field.
func (r *AdminUserRepository) UpdateFinanceRemark(ctx context.Context, id int, remark *string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE admin_user SET finance_remark = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, remark, id)
	if err != nil {
		return fmt.Errorf("update admin finance remark: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpdateCommissionRate updates only the commission rate percentage.
func (r *AdminUserRepository) UpdateCommissionRate(ctx context.Context, id int, rate float64) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE admin_user SET commission_rate = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, rate, id)
	if err != nil {
		return fmt.Errorf("update admin commission rate: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Delete hard-deletes an admin user by ID
func (r *AdminUserRepository) Delete(ctx context.Context, id int) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM admin_user WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete admin: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}
