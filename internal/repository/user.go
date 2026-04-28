package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

// UserRepository handles user database operations
type UserRepository struct {
	db *sqlx.DB
}

// NewUserRepository creates a new UserRepository
func NewUserRepository(db *sqlx.DB) *UserRepository {
	return &UserRepository{db: db}
}

// GetByID retrieves a user by ID with joined school and major info
func (r *UserRepository) GetByID(ctx context.Context, id int) (*models.User, error) {
	query := `
		SELECT
			u.id, u.openid, u.nickname, u.phone, u.email,
			u.school_id, u.major_id, u.grade, u.olive_branch_count,
			u.free_branch_used_today, u.last_active_date,
			u.auth_status, u.auth_img_url, u.avatar_url, u.cover_image,
			u.created_at,
			s.school_name, s.school_code,
			m.major_name, m.class_id
		FROM ` + "`user`" + ` u
		LEFT JOIN school s ON u.school_id = s.id
		LEFT JOIN major m ON u.major_id = m.id
		WHERE u.id = ?
	`

	var user models.User
	if err := r.db.QueryRowxContext(ctx, query, id).StructScan(&user); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query user by id: %w", err)
	}

	return &user, nil
}

// GetByOpenID retrieves a user by WeChat OpenID
func (r *UserRepository) GetByOpenID(ctx context.Context, openid string) (*models.User, error) {
	query := `
		SELECT
			u.id, u.openid, u.nickname, u.phone, u.email,
			u.school_id, u.major_id, u.grade, u.olive_branch_count,
			u.free_branch_used_today, u.last_active_date,
			u.auth_status, u.auth_img_url, u.avatar_url, u.cover_image,
			u.created_at,
			s.school_name, s.school_code,
			m.major_name, m.class_id
		FROM ` + "`user`" + ` u
		LEFT JOIN school s ON u.school_id = s.id
		LEFT JOIN major m ON u.major_id = m.id
		WHERE u.openid = ?
	`

	var user models.User
	if err := r.db.QueryRowxContext(ctx, query, openid).StructScan(&user); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query user by openid: %w", err)
	}

	return &user, nil
}

// Create creates a new user and returns the created user
func (r *UserRepository) Create(ctx context.Context, openid string) (*models.User, error) {
	query := `
		INSERT INTO ` + "`user`" + ` (openid, olive_branch_count, free_branch_used_today, auth_status, created_at)
		VALUES (?, 0, 0, ?, NOW())
	`

	result, err := r.db.ExecContext(ctx, query, openid, models.UserAuthStatusNone)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("get last insert id: %w", err)
	}

	return r.GetByID(ctx, int(id))
}

// CreateWithPhone creates a new user with phone and returns the created user
func (r *UserRepository) CreateWithPhone(ctx context.Context, openid string, phone string) (*models.User, error) {
	query := `
		INSERT INTO ` + "`user`" + ` (openid, phone, olive_branch_count, free_branch_used_today, auth_status, created_at)
		VALUES (?, ?, 0, 0, ?, NOW())
	`

	result, err := r.db.ExecContext(ctx, query, openid, phone, models.UserAuthStatusNone)
	if err != nil {
		return nil, fmt.Errorf("create user with phone: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("get last insert id: %w", err)
	}

	return r.GetByID(ctx, int(id))
}

// Update updates user fields
func (r *UserRepository) Update(ctx context.Context, user *models.User) error {
	query := `
		UPDATE ` + "`user`" + ` SET
			nickname = :nickname,
			phone = :phone,
			email = :email,
			school_id = :school_id,
			major_id = :major_id,
			grade = :grade,
			auth_status = :auth_status
		WHERE id = :id
	`

	_, err := r.db.NamedExecContext(ctx, query, user)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}

	return nil
}

// UpdatePhone updates user phone number
func (r *UserRepository) UpdatePhone(ctx context.Context, userID int, phone string) error {
	query := `UPDATE ` + "`user`" + ` SET phone = ? WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query, phone, userID)
	if err != nil {
		return fmt.Errorf("update user phone: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

// UpdateQuota updates user's olive branch quota fields
func (r *UserRepository) UpdateQuota(ctx context.Context, user *models.User) error {
	query := `
		UPDATE ` + "`user`" + ` SET
			olive_branch_count = :olive_branch_count,
			free_branch_used_today = :free_branch_used_today,
			last_active_date = :last_active_date
		WHERE id = :id
	`

	_, err := r.db.NamedExecContext(ctx, query, user)
	if err != nil {
		return fmt.Errorf("update user quota: %w", err)
	}

	return nil
}

// AddOliveBranchCount atomically adds count to user's olive_branch_count
func (r *UserRepository) AddOliveBranchCount(ctx context.Context, userID int, count int) error {
	query := `
		UPDATE ` + "`user`" + ` SET
			olive_branch_count = olive_branch_count + ?
		WHERE id = ?
	`

	_, err := r.db.ExecContext(ctx, query, count, userID)
	if err != nil {
		return fmt.Errorf("add olive branch count: %w", err)
	}

	return nil
}

// AddOliveBranchCountTx atomically adds count to user's olive_branch_count within a transaction
func (r *UserRepository) AddOliveBranchCountTx(ctx context.Context, tx *sqlx.Tx, userID int, count int) error {
	query := `
		UPDATE ` + "`user`" + ` SET
			olive_branch_count = olive_branch_count + ?
		WHERE id = ?
	`

	_, err := tx.ExecContext(ctx, query, count, userID)
	if err != nil {
		return fmt.Errorf("add olive branch count: %w", err)
	}

	return nil
}

// UpdateAuthStatus updates user's certification auth status
func (r *UserRepository) UpdateAuthStatus(ctx context.Context, userID int, authStatus int) error {
	query := `UPDATE ` + "`user`" + ` SET auth_status = ? WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query, authStatus, userID)
	if err != nil {
		return fmt.Errorf("update auth status: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

// UserListParams contains parameters for listing users
type UserListParams struct {
	Page                int
	Size                int
	AuthStatus          *int
	SchoolID            *int
	Keyword             *string
	AuthImgUploaded     *bool
	TalentProfileStatus *int // 按名片状态过滤（0=已驳回/下架, 1=已上架, 2=审核中）
	UserID              *int // 按用户 ID 精确查询
}

// ListUsers retrieves paginated users with optional filters
func (r *UserRepository) ListUsers(ctx context.Context, params UserListParams) ([]models.User, int64, error) {
	conditions := []string{"1=1"}
	args := []interface{}{}

	if params.AuthStatus != nil {
		conditions = append(conditions, "u.auth_status = ?")
		args = append(args, *params.AuthStatus)
	}

	if params.SchoolID != nil {
		conditions = append(conditions, "u.school_id = ?")
		args = append(args, *params.SchoolID)
	}

	if params.Keyword != nil && *params.Keyword != "" {
		conditions = append(conditions, "(u.nickname LIKE ? OR u.phone LIKE ?)")
		args = append(args, "%"+*params.Keyword+"%", "%"+*params.Keyword+"%")
	}

	if params.AuthImgUploaded != nil {
		if *params.AuthImgUploaded == false {
			conditions = append(conditions, "u.auth_img_url IS NULL")
		} else {
			conditions = append(conditions, "u.auth_img_url IS NOT NULL")
		}
	}

	if params.TalentProfileStatus != nil {
		conditions = append(conditions, "u.id IN (SELECT user_id FROM talent_profile WHERE status = ?)")
		args = append(args, *params.TalentProfileStatus)
	}

	if params.UserID != nil {
		conditions = append(conditions, "u.id = ?")
		args = append(args, *params.UserID)
	}

	whereClause := strings.Join(conditions, " AND ")

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM `user` u WHERE %s", whereClause)
	var total int64
	if err := r.db.QueryRowxContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}

	// Query with pagination
	offset := (params.Page - 1) * params.Size
	query := fmt.Sprintf(`
		SELECT
			u.id, u.openid, u.nickname, u.phone, u.email,
			u.school_id, u.major_id, u.grade, u.olive_branch_count,
			u.free_branch_used_today, u.last_active_date,
			u.auth_status, u.auth_img_url, u.avatar_url, u.cover_image, u.created_at,
			s.school_name, s.school_code,
			m.major_name, m.class_id
		FROM `+"`user`"+` u
		LEFT JOIN school s ON u.school_id = s.id
		LEFT JOIN major m ON u.major_id = m.id
		WHERE %s
		ORDER BY u.created_at DESC
		LIMIT ? OFFSET ?
	`, whereClause)
	args = append(args, params.Size, offset)

	var users []models.User
	if err := r.db.SelectContext(ctx, &users, query, args...); err != nil {
		return nil, 0, fmt.Errorf("query users: %w", err)
	}

	return users, total, nil
}

// EmailRecipient 邮件接收者
type EmailRecipient struct {
	ID       int     `db:"id"`
	Email    string  `db:"email"`
	Nickname *string `db:"nickname"`
}

// FindEmailRecipients 查找邮件发送对象
// 排除指定用户，排除已退订用户，随机排序后限制数量
func (r *UserRepository) FindEmailRecipients(ctx context.Context, excludeUserID int, limit int) ([]*EmailRecipient, error) {
	query := `
		SELECT id, email, nickname 
		FROM ` + "`user`" + `
		WHERE email IS NOT NULL 
		  AND email != ''
		  AND email_opt_out = FALSE
		  AND id != ?
		ORDER BY RAND()
		LIMIT ?
	`

	var recipients []*EmailRecipient
	if err := r.db.SelectContext(ctx, &recipients, query, excludeUserID, limit); err != nil {
		return nil, fmt.Errorf("query email recipients: %w", err)
	}

	return recipients, nil
}

// SetEmailOptOut 设置用户的邮件退订状态
func (r *UserRepository) SetEmailOptOut(ctx context.Context, userID int, optOut bool) error {
	query := `
		UPDATE ` + "`user`" + ` SET
			email_opt_out = ?
		WHERE id = ?
	`

	_, err := r.db.ExecContext(ctx, query, optOut, userID)
	if err != nil {
		return fmt.Errorf("set email opt out: %w", err)
	}
	return nil
}

// UpdateAuthImgUrl updates user's authentication image URL
func (r *UserRepository) UpdateAuthImgUrl(ctx context.Context, userID int, authImgUrl string) error {
	query := `
		UPDATE ` + "`user`" + ` SET
			auth_img_url = ?
		WHERE id = ?
	`

	_, err := r.db.ExecContext(ctx, query, authImgUrl, userID)
	if err != nil {
		return fmt.Errorf("update user auth img url: %w", err)
	}

	return nil
}

// UpdateAvatarUrl updates user's avatar URL
func (r *UserRepository) UpdateAvatarUrl(ctx context.Context, userID int, avatarUrl string) error {
	query := `UPDATE ` + "`user`" + ` SET avatar_url = ? WHERE id = ?`

	_, err := r.db.ExecContext(ctx, query, avatarUrl, userID)
	if err != nil {
		return fmt.Errorf("update user avatar url: %w", err)
	}
	return nil
}

// UpdateCoverImage updates user's cover image URL
func (r *UserRepository) UpdateCoverImage(ctx context.Context, userID int, coverImage string) error {
	query := `UPDATE ` + "`user`" + ` SET cover_image = ? WHERE id = ?`

	_, err := r.db.ExecContext(ctx, query, coverImage, userID)
	if err != nil {
		return fmt.Errorf("update user cover image: %w", err)
	}
	return nil
}

type CertInfo struct {
	Status     int     `db:"auth_status"`
	AuthImgUrl *string `db:"auth_img_url"`
}

func (r *UserRepository) GetEduCertInfoByID(ctx context.Context, userID int) (CertInfo, error) {
	query := `
		SELECT auth_status, auth_img_url 
		FROM ` + "`user`" + `
		WHERE id = ?
	`

	var info CertInfo
	if err := r.db.QueryRowxContext(ctx, query, userID).StructScan(&info); err != nil {
		return CertInfo{}, fmt.Errorf("get auth status: %w", err)
	}

	return info, nil
}
