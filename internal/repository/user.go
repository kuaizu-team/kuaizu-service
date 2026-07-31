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
			u.wechat_id, u.sent_olive_viewed_at, u.applications_last_viewed_at, u.last_viewed_my_projects_at,
			u.user_status, u.ban_reason, ucg.status AS competition_group_status, ucg.note AS competition_group_note, u.collaboration_score, u.created_at,
			s.school_name, s.school_code,
			m.major_name, m.class_id
		FROM ` + "`user`" + ` u
		LEFT JOIN school s ON u.school_id = s.id
		LEFT JOIN major m ON u.major_id = m.id
        LEFT JOIN user_competition_group ucg ON ucg.user_id = u.id
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

// GetByPhone retrieves a user by exact phone number with safe display joins.
func (r *UserRepository) GetByPhone(ctx context.Context, phone string) (*models.User, error) {
	query := `
		SELECT
			u.id, u.openid, u.nickname, u.phone, u.email,
			u.school_id, u.major_id, u.grade,
			u.auth_status, u.avatar_url, u.collaboration_score, u.created_at,
			s.school_name,
			m.major_name,
			tp.id AS talent_profile_id
		FROM ` + "`user`" + ` u
		LEFT JOIN school s ON u.school_id = s.id
		LEFT JOIN major m ON u.major_id = m.id
		LEFT JOIN talent_profile tp ON u.id = tp.user_id
		WHERE u.phone = ?
		LIMIT 1
	`

	var user models.User
	if err := r.db.QueryRowxContext(ctx, query, phone).StructScan(&user); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query user by phone: %w", err)
	}

	return &user, nil
}

// GetByIDForUpdateTx retrieves and locks a user row in the current transaction.
func (r *UserRepository) GetByIDForUpdateTx(ctx context.Context, tx *sqlx.Tx, id int) (*models.User, error) {
	query := `
		SELECT
			u.id, u.openid, u.nickname, u.phone, u.email,
			u.school_id, u.major_id, u.grade, u.olive_branch_count,
			u.free_branch_used_today, u.last_active_date,
			u.auth_status, u.auth_img_url, u.avatar_url, u.cover_image,
			u.wechat_id, u.sent_olive_viewed_at, u.applications_last_viewed_at, u.last_viewed_my_projects_at,
			u.user_status, u.ban_reason, ucg.status AS competition_group_status, ucg.note AS competition_group_note, u.collaboration_score, u.created_at,
			s.school_name, s.school_code,
			m.major_name, m.class_id
		FROM ` + "`user`" + ` u
		LEFT JOIN school s ON u.school_id = s.id
		LEFT JOIN major m ON u.major_id = m.id
        LEFT JOIN user_competition_group ucg ON ucg.user_id = u.id
		WHERE u.id = ?
		FOR UPDATE
	`

	var user models.User
	if err := tx.QueryRowxContext(ctx, query, id).StructScan(&user); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query user by id for update: %w", err)
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
			u.wechat_id, u.sent_olive_viewed_at, u.applications_last_viewed_at, u.last_viewed_my_projects_at,
			u.user_status, u.ban_reason, ucg.status AS competition_group_status, ucg.note AS competition_group_note, u.collaboration_score, u.created_at,
			s.school_name, s.school_code,
			m.major_name, m.class_id
		FROM ` + "`user`" + ` u
		LEFT JOIN school s ON u.school_id = s.id
		LEFT JOIN major m ON u.major_id = m.id
        LEFT JOIN user_competition_group ucg ON ucg.user_id = u.id
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

// Update updates user fields.
func (r *UserRepository) Update(ctx context.Context, user *models.User) error {
	return updateUser(ctx, r.db, user)
}

// UpdateUserTx updates user fields in an existing transaction.
func UpdateUserTx(ctx context.Context, tx *sqlx.Tx, user *models.User) error {
	return updateUser(ctx, tx, user)
}

func updateUser(ctx context.Context, exec sqlx.ExtContext, user *models.User) error {
	query := `
		UPDATE ` + "`user`" + ` SET
			nickname = :nickname,
			phone = :phone,
			email = :email,
			school_id = :school_id,
			major_id = :major_id,
			grade = :grade,
			auth_status = :auth_status,
			wechat_id = :wechat_id
		WHERE id = :id
	`

	if _, err := sqlx.NamedExecContext(ctx, exec, query, user); err != nil {
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

// UpdateQuotaTx updates user's olive branch quota fields within a transaction.
func (r *UserRepository) UpdateQuotaTx(ctx context.Context, tx *sqlx.Tx, user *models.User) error {
	query := `
		UPDATE ` + "`user`" + ` SET
			olive_branch_count = :olive_branch_count,
			free_branch_used_today = :free_branch_used_today,
			last_active_date = :last_active_date
		WHERE id = :id
	`

	_, err := tx.NamedExecContext(ctx, query, user)
	if err != nil {
		return fmt.Errorf("update user quota: %w", err)
	}

	return nil
}

// ResetDailyFreeBranchQuotaIfNeeded resets daily free olive branch usage when a new Asia/Shanghai calendar day starts.
func (r *UserRepository) ResetDailyFreeBranchQuotaIfNeeded(ctx context.Context, userID int) error {
	query := `
		UPDATE ` + "`user`" + `
		SET free_branch_used_today = 0,
			last_active_date = DATE(UTC_TIMESTAMP() + INTERVAL 8 HOUR)
		WHERE id = ? AND (last_active_date IS NULL OR last_active_date < DATE(UTC_TIMESTAMP() + INTERVAL 8 HOUR))
	`
	_, err := r.db.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("reset daily free branch quota: %w", err)
	}
	return nil
}

// ResetDailyFreeBranchQuotaIfNeededTx resets daily free olive branch usage within a transaction.
func (r *UserRepository) ResetDailyFreeBranchQuotaIfNeededTx(ctx context.Context, tx *sqlx.Tx, userID int) error {
	query := `
		UPDATE ` + "`user`" + `
		SET free_branch_used_today = 0,
			last_active_date = DATE(UTC_TIMESTAMP() + INTERVAL 8 HOUR)
		WHERE id = ? AND (last_active_date IS NULL OR last_active_date < DATE(UTC_TIMESTAMP() + INTERVAL 8 HOUR))
	`
	_, err := tx.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("reset daily free branch quota: %w", err)
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

// ResubmitCertification atomically updates auth_img_url and resets auth_status to 0
// only when the current status is 2 (failed). Status 0 and 1 are left unchanged.
func (r *UserRepository) ResubmitCertification(ctx context.Context, userID int, imgKey string) error {
	query := `
		UPDATE ` + "`user`" + ` SET
			auth_img_url = ?,
			auth_status  = CASE WHEN auth_status = 2 THEN 0 ELSE auth_status END
		WHERE id = ?
	`
	_, err := r.db.ExecContext(ctx, query, imgKey, userID)
	if err != nil {
		return fmt.Errorf("resubmit certification: %w", err)
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

// InvitationFeedbackEffectiveStatusSQL normalizes the feedback and conversation
// columns to the single status shown in the admin user list. It expects the
// invitation_feedback and pending_invitation aliases to be f and p.
const InvitationFeedbackEffectiveStatusSQL = `CASE
	WHEN f.conversation_status = 'in_progress' AND p.user_id IS NOT NULL THEN 'in_progress'
	WHEN f.conversation_status = 'in_progress' THEN 'pending'
	WHEN f.conversation_status IS NOT NULL THEN f.conversation_status
	ELSE f.status
END`

// UserListParams contains parameters for listing users
type UserListParams struct {
	Page                int
	Size                int
	AuthStatus          *int
	SchoolID            *int
	SchoolIDs           []int
	Keyword             *string
	AuthImgUploaded     *bool
	TalentProfileStatus *int // 按名片状态过滤（0=已驳回/下架, 1=已上架, 2=审核中）
	UserID              *int // 按用户 ID 精确查询

	UserStatus              *int    // 按账号状态过滤（0=正常, 1=封禁, 2=已毕业）
	InvitationFeedbackStatus *string // 按列表展示的邀请反馈状态过滤

	// Admin sort control
	// SortBy: "pendingCount" | "lastActiveDate" — unknown values fall back to created_at DESC
	SortBy *string
	// Order: "asc" | "desc" (case-insensitive). Defaults to DESC.
	Order *string
	// IncludePendingCount — when true, adds correlated subqueries that compute
	// pending_count = (project_application WHERE user_id=u.id AND status=0)   -- 用户投出的待审核投递
	//               + (olive_branch_record WHERE receiver_id=u.id AND status=0) -- 用户收到的待处理橄榄枝
	IncludePendingCount bool
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
	if len(params.SchoolIDs) > 0 {
		condition, inArgs, err := sqlx.In("u.school_id IN (?)", params.SchoolIDs)
		if err != nil {
			return nil, 0, fmt.Errorf("build user school filter: %w", err)
		}
		conditions = append(conditions, condition)
		args = append(args, inArgs...)
	} else if params.SchoolIDs != nil {
		conditions = append(conditions, "1=0")
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
		if *params.TalentProfileStatus == -1 {
			// -1 = 从未提交名片（无任何名片记录）
			conditions = append(conditions, "u.id NOT IN (SELECT user_id FROM talent_profile)")
		} else {
			conditions = append(conditions, "u.id IN (SELECT user_id FROM talent_profile WHERE status = ?)")
			args = append(args, *params.TalentProfileStatus)
		}
	}

	if params.UserID != nil {
		conditions = append(conditions, "u.id = ?")
		args = append(args, *params.UserID)
	}

	if params.UserStatus != nil {
		conditions = append(conditions, "u.user_status = ?")
		args = append(args, *params.UserStatus)
	}

	if params.InvitationFeedbackStatus != nil {
		conditions = append(conditions, `EXISTS (
			SELECT 1
			FROM invitation_feedback f
			LEFT JOIN pending_invitation p
				ON p.user_id = f.user_id
				AND p.invite_type = 'SUPER_ADMIN'
				AND p.expire_at > CURRENT_TIMESTAMP
			WHERE f.user_id = u.id
				AND `+InvitationFeedbackEffectiveStatusSQL+` = ?
		)`)
		args = append(args, *params.InvitationFeedbackStatus)
	}

	whereClause := strings.Join(conditions, " AND ")

	// Count total — WHERE args only, no ORDER BY subqueries involved
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM `user` u WHERE %s", whereClause)
	var total int64
	if err := r.db.QueryRowxContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}

	// Build ORDER BY
	// - "pendingCount"    → pending_count [dir], u.created_at DESC
	// - "lastActiveDate"  → u.last_active_date [dir], u.created_at DESC
	// - default           → u.created_at DESC
	orderClause := "u.created_at DESC"
	if params.SortBy != nil {
		dir := "DESC"
		if params.Order != nil && strings.EqualFold(*params.Order, "asc") {
			dir = "ASC"
		}
		switch *params.SortBy {
		case "pendingCount":
			orderClause = fmt.Sprintf("pending_count %s, u.created_at DESC", dir)
		case "lastActiveDate":
			orderClause = fmt.Sprintf("u.last_active_date %s, u.created_at DESC", dir)
		case "id":
			orderClause = fmt.Sprintf("u.id %s", dir)
		}
		// unknown sortBy values fall through to default (no error)
	}

	// When IncludePendingCount=true, add correlated subqueries that compute:
	//   pending_count = COUNT(project_application WHERE user_id=u.id AND status=0)   -- 待审核投递
	//                 + COUNT(olive_branch_record WHERE receiver_id=u.id AND status=0) -- 待处理橄榄枝
	// COUNT(*) naturally returns 0 when no rows match, so COALESCE is not strictly needed
	// but kept for defensive consistency.
	pendingCountSelect := ""
	if params.IncludePendingCount {
		pendingCountSelect = `,
			COALESCE((SELECT COUNT(*) FROM project_application WHERE user_id = u.id AND status = 0), 0)
			+ COALESCE((SELECT COUNT(*) FROM olive_branch_record WHERE receiver_id = u.id AND status = 0), 0)
			AS pending_count`
	}

	// Query with pagination
	offset := (params.Page - 1) * params.Size
	query := fmt.Sprintf(`
		SELECT
			u.id, u.openid, u.nickname, u.phone, u.email,
			u.school_id, u.major_id, u.grade, u.olive_branch_count,
			u.free_branch_used_today, u.last_active_date,
			u.auth_status, u.auth_img_url, u.avatar_url, u.cover_image,
			u.wechat_id, u.user_status, u.ban_reason, ucg.status AS competition_group_status, ucg.note AS competition_group_note, u.collaboration_score, u.created_at,
			s.school_name, s.school_code,
			m.major_name, m.class_id%s
		FROM `+"`user`"+` u
		LEFT JOIN school s ON u.school_id = s.id
		LEFT JOIN major m ON u.major_id = m.id
        LEFT JOIN user_competition_group ucg ON ucg.user_id = u.id
		WHERE %s
		ORDER BY %s
		LIMIT ? OFFSET ?
	`, pendingCountSelect, whereClause, orderClause)
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

// UpdateApplicationsLastViewedAt sets applications_last_viewed_at to NOW() for the given user.
func (r *UserRepository) UpdateApplicationsLastViewedAt(ctx context.Context, userID int) error {
	query := `UPDATE ` + "`user`" + ` SET applications_last_viewed_at = NOW() WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("update applications_last_viewed_at: %w", err)
	}
	return nil
}

// UpdateLastViewedMyProjectsAt sets last_viewed_my_projects_at to NOW() for the given user.
func (r *UserRepository) UpdateLastViewedMyProjectsAt(ctx context.Context, userID int) error {
	query := `UPDATE ` + "`user`" + ` SET last_viewed_my_projects_at = NOW() WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("update last_viewed_my_projects_at: %w", err)
	}
	return nil
}

// UpdateSentOliveViewedAt sets sent_olive_viewed_at to the current timestamp for the given user.
func (r *UserRepository) UpdateSentOliveViewedAt(ctx context.Context, userID int) error {
	query := `UPDATE ` + "`user`" + ` SET sent_olive_viewed_at = NOW() WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("update sent_olive_viewed_at: %w", err)
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

// TouchLastActiveDate updates last_active_date to today and resets daily free
// olive branch usage when the stored value is NULL or earlier than today.
// This is designed to be called on every user login / app launch.
func (r *UserRepository) TouchLastActiveDate(ctx context.Context, userID int) error {
	return r.ResetDailyFreeBranchQuotaIfNeeded(ctx, userID)
}

// UpdateCompetitionGroup stores the admin-managed competition group status.
func (r *UserRepository) UpdateCompetitionGroup(ctx context.Context, userID int, status *string, note *string, adminID int) error {
	if status == nil {
		note = nil
	}
	query := `
		INSERT INTO user_competition_group (user_id, status, note, updated_by_admin_id, updated_at)
		VALUES (?, ?, ?, ?, NOW())
		ON DUPLICATE KEY UPDATE
			status = VALUES(status),
			note = VALUES(note),
			updated_by_admin_id = VALUES(updated_by_admin_id),
			updated_at = NOW()
	`
	if _, err := r.db.ExecContext(ctx, query, userID, status, note, adminID); err != nil {
		return fmt.Errorf("update user competition group: %w", err)
	}
	return nil
}

// UpdateUserStatus updates user_status and ban_reason for the given user.
// banReason is set to NULL when status != 1.
func (r *UserRepository) UpdateUserStatus(ctx context.Context, userID int, status int, banReason *string) error {
	reason := banReason
	if status != 1 {
		reason = nil
	}
	query := `UPDATE ` + "`user`" + ` SET user_status = ?, ban_reason = ? WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, status, reason, userID)
	if err != nil {
		return fmt.Errorf("update user status: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
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
