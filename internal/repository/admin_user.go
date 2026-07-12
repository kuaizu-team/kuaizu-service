package repository

import (
	"context"
	"database/sql"
	"fmt"
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
	au.id, au.username, au.password_hash, au.nickname,
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

	return &admin, nil
}

// AdminUserListParams contains parameters for listing admin users
type AdminUserListParams struct {
	Page     int
	Size     int
	Keyword  *string // 按 username/nickname 模糊搜索
	Role     *int    // 按角色筛选（1/2/3）
	Status   *int    // 按状态筛选（0/1）
	SchoolID *int    // 校区管理员只能看本校，超级管理员不传
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
		conditions = append(conditions, "au.school_id = ?")
		args = append(args, *params.SchoolID)
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

	return admins, total, nil
}

// ErrDuplicateUsername is returned when a username conflicts with an existing record
var ErrDuplicateUsername = fmt.Errorf("账号已存在")

// Create inserts a new admin user and populates its ID
func (r *AdminUserRepository) Create(ctx context.Context, admin *models.AdminUser) error {
	query := `
		INSERT INTO admin_user (username, password_hash, nickname, role, school_id, status)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	result, err := r.db.ExecContext(ctx, query,
		admin.Username, admin.PasswordHash, admin.Nickname,
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
			SET nickname = ?, role = ?, school_id = ?, status = ?, join_date = ?, intro = ?, article_url = ?, password_hash = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?`
		args = []interface{}{admin.Nickname, admin.Role, admin.SchoolID, admin.Status, admin.JoinDate, admin.Intro, admin.ArticleURL, admin.PasswordHash, admin.ID}
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
