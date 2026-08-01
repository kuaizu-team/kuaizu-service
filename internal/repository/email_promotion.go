package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/oss"
)

const promotionRecipientRecentDays = 30

const emailPromotionSelectColumns = `
	ep.id, ep.channel, ep.business_tag, ep.trace_id,
	ep.order_id, ep.project_id, ep.creator_id,
	ep.strategy, ep.max_recipients, ep.total_sent, ep.status,
	ep.error_message, ep.started_at, ep.completed_at, ep.created_at
`

const emailPromotionUpdateQuery = `
	UPDATE email_promotion SET
		channel = :channel,
		business_tag = :business_tag,
		trace_id = :trace_id,
		project_id = :project_id,
		creator_id = :creator_id,
		strategy = :strategy,
		max_recipients = :max_recipients,
		total_sent = :total_sent,
		status = :status,
		error_message = :error_message,
		started_at = :started_at,
		completed_at = :completed_at
	WHERE id = :id
`

const emailPromotionMetadataUpdateQuery = `
	UPDATE email_promotion SET
		channel = :channel,
		business_tag = :business_tag,
		trace_id = :trace_id,
		project_id = :project_id,
		creator_id = :creator_id,
		strategy = :strategy,
		max_recipients = :max_recipients
	WHERE id = :id
`

// EmailPromotionRepository handles email promotion database operations
type EmailPromotionRepository struct {
	db *sqlx.DB
}

// NewEmailPromotionRepository creates a new EmailPromotionRepository
func NewEmailPromotionRepository(db *sqlx.DB) *EmailPromotionRepository {
	return &EmailPromotionRepository{db: db}
}

// Create creates or updates the single promotion record for an order/project pair.
func (r *EmailPromotionRepository) Create(ctx context.Context, promotion *models.EmailPromotion) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin email promotion create: %w", err)
	}
	defer tx.Rollback()

	var lockedOrderID int
	if err := tx.GetContext(ctx, &lockedOrderID,
		"SELECT id FROM `order` WHERE id = ? FOR UPDATE", promotion.OrderID); err != nil {
		return fmt.Errorf("lock email promotion order: %w", err)
	}

	existing, err := getPreferredByOrderAndProjectTx(ctx, tx, promotion.OrderID, promotion.ProjectID)
	if err != nil {
		return err
	}
	if existing != nil {
		promotion.ID = existing.ID
		if _, err := tx.NamedExecContext(ctx, emailPromotionMetadataUpdateQuery, promotion); err != nil {
			return fmt.Errorf("update existing email promotion: %w", err)
		}
		promotion.TotalSent = existing.TotalSent
		promotion.Status = existing.Status
		promotion.ErrorMessage = existing.ErrorMessage
		promotion.StartedAt = existing.StartedAt
		promotion.CompletedAt = existing.CompletedAt
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit existing email promotion: %w", err)
		}
		return nil
	}

	query := `
		INSERT INTO email_promotion (
			channel, business_tag, trace_id,
			order_id, project_id, creator_id, strategy, max_recipients, total_sent, status
		) VALUES (
			:channel, :business_tag, :trace_id,
			:order_id, :project_id, :creator_id, :strategy, :max_recipients, :total_sent, :status
		)
	`

	result, err := tx.NamedExecContext(ctx, query, promotion)
	if err != nil {
		return fmt.Errorf("create email promotion: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}

	promotion.ID = int(id)
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit email promotion create: %w", err)
	}
	return nil
}

func getPreferredByOrderAndProjectTx(ctx context.Context, tx *sqlx.Tx, orderID, projectID int) (*models.EmailPromotion, error) {
	query := preferredEmailPromotionQuery()
	var promotion models.EmailPromotion
	if err := tx.QueryRowxContext(ctx, query, orderID, projectID).StructScan(&promotion); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get preferred email promotion in transaction: %w", err)
	}
	return &promotion, nil
}

func (r *EmailPromotionRepository) getPreferredByOrderAndProject(ctx context.Context, orderID, projectID int) (*models.EmailPromotion, error) {
	query := preferredEmailPromotionQuery()

	var promotion models.EmailPromotion
	if err := r.db.QueryRowxContext(ctx, query, orderID, projectID).StructScan(&promotion); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get preferred email promotion: %w", err)
	}
	return &promotion, nil
}

func preferredEmailPromotionQuery() string {
	return `
		SELECT ` + emailPromotionSelectColumns + `
		FROM email_promotion ep
		WHERE ep.order_id = ? AND ep.project_id = ?
		ORDER BY ` + promotionRealRank("ep") + ` ASC,
		         ` + promotionPromotedAtExpr("ep") + ` DESC,
		         ` + promotionStatusRank("ep") + ` ASC,
		         ep.created_at DESC,
		         ep.id DESC
		LIMIT 1
	`
}

// GetByID retrieves an email promotion by ID
func (r *EmailPromotionRepository) GetByID(ctx context.Context, id int) (*models.EmailPromotion, error) {
	query := `
		SELECT 
			` + emailPromotionSelectColumns + `,
			p.name AS project_name
		FROM email_promotion ep
		LEFT JOIN project p ON ep.project_id = p.id
		WHERE ep.id = ?
	`

	var promotion models.EmailPromotion
	if err := r.db.QueryRowxContext(ctx, query, id).StructScan(&promotion); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get email promotion by id: %w", err)
	}

	return &promotion, nil
}

// GetByOrderID retrieves an email promotion by order ID
func (r *EmailPromotionRepository) GetByOrderID(ctx context.Context, orderID int) (*models.EmailPromotion, error) {
	query := `
		SELECT 
			` + emailPromotionSelectColumns + `
		FROM email_promotion ep
		WHERE ep.order_id = ?
		ORDER BY ` + promotionRealRank("ep") + ` ASC,
		         ` + promotionPromotedAtExpr("ep") + ` DESC,
		         ` + promotionStatusRank("ep") + ` ASC,
		         ep.created_at DESC,
		         ep.id DESC
		LIMIT 1
	`

	var promotion models.EmailPromotion
	if err := r.db.QueryRowxContext(ctx, query, orderID).StructScan(&promotion); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get email promotion by order id: %w", err)
	}

	return &promotion, nil
}

// GetByOrderAndProject retrieves the preferred promotion record for an order/project pair.
func (r *EmailPromotionRepository) GetByOrderAndProject(ctx context.Context, orderID, projectID int) (*models.EmailPromotion, error) {
	return r.getPreferredByOrderAndProject(ctx, orderID, projectID)
}

// Update updates an email promotion record
func (r *EmailPromotionRepository) Update(ctx context.Context, promotion *models.EmailPromotion) error {
	_, err := r.db.NamedExecContext(ctx, emailPromotionUpdateQuery, promotion)
	if err != nil {
		return fmt.Errorf("update email promotion: %w", err)
	}

	return nil
}

// UpdateMetadata normalizes routing metadata without writing message-center-owned execution state.
func (r *EmailPromotionRepository) UpdateMetadata(ctx context.Context, promotion *models.EmailPromotion) error {
	_, err := r.db.NamedExecContext(ctx, emailPromotionMetadataUpdateQuery, promotion)
	if err != nil {
		return fmt.Errorf("update email promotion metadata: %w", err)
	}

	return nil
}

// MarkFailedIfNotCompleted conditionally records failure without downgrading a completed promotion.
func (r *EmailPromotionRepository) MarkFailedIfNotCompleted(ctx context.Context, promotionID int, message string, completedAt time.Time) (bool, error) {
	result, err := r.db.ExecContext(ctx, `UPDATE email_promotion SET
		status=?, error_message=?, completed_at=?
		WHERE id=? AND (status IS NULL OR status<>?)`,
		models.EmailPromotionStatusFailed, message, completedAt, promotionID, models.EmailPromotionStatusCompleted)
	if err != nil {
		return false, fmt.Errorf("conditionally fail email promotion: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read failed email promotion affected rows: %w", err)
	}
	return affected == 1, nil
}

// CreateRecipients stores the selected recipient users for a promotion batch.
func (r *EmailPromotionRepository) CreateRecipients(ctx context.Context, promotionID, projectID int, userIDs []int) error {
	if len(userIDs) == 0 {
		return nil
	}

	rows := make([]string, 0, len(userIDs))
	args := make([]interface{}, 0, len(userIDs)*3)
	for _, userID := range userIDs {
		rows = append(rows, "(?, ?, ?, 0)")
		args = append(args, promotionID, projectID, userID)
	}

	query := `
		INSERT IGNORE INTO email_promotion_recipient (
			promotion_id, project_id, user_id, status
		) VALUES ` + strings.Join(rows, ",")
	if _, err := r.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("create promotion recipients: %w", err)
	}
	return nil
}

// GetRetryRecipientUserIDs returns the original recipient set for a retry.
// Current batches use the immutable snapshot; legacy batches fall back to sent task addresses.
func (r *EmailPromotionRepository) GetRetryRecipientUserIDs(ctx context.Context, promotionID, limit int) ([]int, error) {
	if limit <= 0 {
		return nil, nil
	}

	var userIDs []int
	if err := r.db.SelectContext(ctx, &userIDs, `
		SELECT user_id
		FROM email_promotion_recipient
		WHERE promotion_id = ?
		ORDER BY created_at ASC, id ASC
		LIMIT ?`, promotionID, limit); err != nil {
		return nil, fmt.Errorf("query promotion recipient snapshot: %w", err)
	}
	if len(userIDs) > 0 {
		return userIDs, nil
	}

	if err := r.db.SelectContext(ctx, &userIDs, `
		SELECT MIN(u.id) AS id
		FROM (
			SELECT LOWER(TRIM(recipient_email)) AS recipient_email, MIN(id) AS first_task_id
			FROM email_task
			WHERE promotion_id = ?
			  AND recipient_email IS NOT NULL
			  AND TRIM(recipient_email) <> ''
			GROUP BY LOWER(TRIM(recipient_email))
		) et
		JOIN `+"`user`"+` u ON LOWER(TRIM(u.email)) = et.recipient_email
		WHERE u.email IS NOT NULL AND TRIM(u.email) <> ''
		GROUP BY et.recipient_email, et.first_task_id
		ORDER BY et.first_task_id ASC
		LIMIT ?`, promotionID, limit); err != nil {
		return nil, fmt.Errorf("query legacy promotion recipients: %w", err)
	}
	return userIDs, nil
}

// SelectPromotionRecipients chooses recipient users by strategy and priority tiers.
func (r *EmailPromotionRepository) SelectPromotionRecipients(ctx context.Context, projectID, creatorID int, strategy string, limit int) ([]int, error) {
	if limit <= 0 {
		return nil, nil
	}

	var tiers []string
	switch strategy {
	case "region":
		otherSchool := "(p.school_id IS NULL OR u.school_id IS NULL OR u.school_id <> p.school_id)"
		tiers = []string{
			"u.school_id = p.school_id AND COALESCE(u.auth_status, 0) = 1",
			"u.school_id = p.school_id AND COALESCE(u.auth_status, 0) <> 1",
			otherSchool + " AND ps.district IS NOT NULL AND ps.district <> '' AND us.district = ps.district AND us.city = ps.city AND COALESCE(u.auth_status, 0) = 1",
			otherSchool + " AND ps.city IS NOT NULL AND ps.city <> '' AND us.city = ps.city AND COALESCE(u.auth_status, 0) = 1",
			otherSchool + " AND ps.province IS NOT NULL AND ps.province <> '' AND us.province = ps.province AND COALESCE(u.auth_status, 0) = 1",
			otherSchool + " AND COALESCE(u.auth_status, 0) = 1",
			otherSchool + " AND COALESCE(u.auth_status, 0) <> 1",
		}
	case "project":
		tiers = []string{
			"COALESCE(u.auth_status, 0) = 1",
			"COALESCE(u.auth_status, 0) <> 1",
		}
	case "major":
		tiers = []string{
			"cm.class_id IS NOT NULL AND m.class_id = cm.class_id AND COALESCE(u.auth_status, 0) = 1",
			"cm.class_id IS NOT NULL AND m.class_id = cm.class_id AND COALESCE(u.auth_status, 0) <> 1",
			"(cm.class_id IS NULL OR m.class_id IS NULL OR m.class_id <> cm.class_id) AND COALESCE(u.auth_status, 0) = 1",
			"(cm.class_id IS NULL OR m.class_id IS NULL OR m.class_id <> cm.class_id) AND COALESCE(u.auth_status, 0) <> 1",
		}
	default:
		return nil, fmt.Errorf("unsupported promotion strategy: %s", strategy)
	}

	selected := make([]int, 0, limit)
	seen := make(map[int]struct{}, limit)
	for _, tier := range tiers {
		if len(selected) >= limit {
			break
		}
		remaining := limit - len(selected)
		ids, err := r.selectPromotionTier(ctx, projectID, creatorID, tier, selected, remaining)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			selected = append(selected, id)
		}
	}
	return selected, nil
}

func (r *EmailPromotionRepository) selectPromotionTier(ctx context.Context, projectID, creatorID int, tier string, excluded []int, limit int) ([]int, error) {
	query := `
		SELECT u.id
		FROM ` + "`user`" + ` u
		JOIN project p ON p.id = ?
		LEFT JOIN school ps ON p.school_id = ps.id
		LEFT JOIN school us ON u.school_id = us.id
		LEFT JOIN ` + "`user`" + ` cu ON cu.id = p.creator_id
		LEFT JOIN major cm ON cu.major_id = cm.id
		LEFT JOIN major m ON u.major_id = m.id
		WHERE u.id <> ?
		  AND u.email IS NOT NULL AND u.email <> ''
		  AND COALESCE(u.email_opt_out, 0) = 0
		  AND NOT EXISTS (
			  SELECT 1
			  FROM email_promotion_recipient recent
			  WHERE recent.project_id = ?
			    AND recent.user_id = u.id
			    AND recent.created_at >= NOW() - INTERVAL ? DAY
		  )
		  AND (` + tier + `)`

	args := []interface{}{projectID, creatorID, projectID, promotionRecipientRecentDays}
	if len(excluded) > 0 {
		query += " AND u.id NOT IN (?)"
		args = append(args, excluded)
	}
	query += " ORDER BY RAND() LIMIT ?"
	args = append(args, limit)

	query, args, err := sqlx.In(query, args...)
	if err != nil {
		return nil, fmt.Errorf("build promotion tier query: %w", err)
	}
	query = r.db.Rebind(query)

	var ids []int
	if err := r.db.SelectContext(ctx, &ids, query, args...); err != nil {
		return nil, fmt.Errorf("query promotion recipients: %w", err)
	}
	return ids, nil
}

// ListByCreatorID retrieves email promotions by creator ID with pagination
func (r *EmailPromotionRepository) ListByCreatorID(ctx context.Context, creatorID int, page, size int) ([]models.EmailPromotion, int64, error) {
	// Count total
	countQuery := `
		SELECT COUNT(*)
		FROM email_promotion ep
		WHERE ep.creator_id = ?
		  AND ` + bestPromotionPredicate("ep", "other") + `
	`
	var total int64
	if err := r.db.QueryRowxContext(ctx, countQuery, creatorID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count email promotions: %w", err)
	}

	// Query with pagination
	offset := (page - 1) * size
	query := `
		SELECT 
			` + emailPromotionSelectColumns + `,
			p.name AS project_name
		FROM email_promotion ep
		LEFT JOIN project p ON ep.project_id = p.id
		WHERE ep.creator_id = ?
		  AND ` + bestPromotionPredicate("ep", "other") + `
		ORDER BY ep.created_at DESC
		LIMIT ? OFFSET ?
	`

	var promotions []models.EmailPromotion
	if err := r.db.SelectContext(ctx, &promotions, query, creatorID, size, offset); err != nil {
		return nil, 0, fmt.Errorf("query email promotions: %w", err)
	}

	return promotions, total, nil
}

// ListByProjectID retrieves email promotions by project ID
func (r *EmailPromotionRepository) ListByProjectID(ctx context.Context, projectID int) ([]models.EmailPromotion, error) {
	query := `
		SELECT 
			` + emailPromotionSelectColumns + `
		FROM email_promotion ep
		WHERE ep.project_id = ?
		  AND ` + bestPromotionPredicate("ep", "other") + `
		ORDER BY ep.created_at DESC
	`

	var promotions []models.EmailPromotion
	if err := r.db.SelectContext(ctx, &promotions, query, projectID); err != nil {
		return nil, fmt.Errorf("query email promotions by project: %w", err)
	}

	return promotions, nil
}

// ListByProjectSince retrieves recent email promotions for a project.
func (r *EmailPromotionRepository) ListByProjectSince(ctx context.Context, projectID, days, limit int) ([]models.EmailPromotion, int64, error) {
	if limit <= 0 {
		limit = 10
	}
	return r.ListByProjectPaged(ctx, projectID, 1, limit, days, limit)
}

// ListByProjectPaged retrieves promotion batches by project with page/size and optional legacy filters.
func (r *EmailPromotionRepository) ListByProjectPaged(ctx context.Context, projectID, page, size, days, limit int) ([]models.EmailPromotion, int64, error) {
	conditions := []string{"ep.project_id = ?"}
	args := []interface{}{projectID}
	if days > 0 {
		conditions = append(conditions, "ep.created_at >= NOW() - INTERVAL ? DAY")
		args = append(args, days)
	}
	conditions = append(conditions, bestPromotionPredicate("ep", "other"))
	whereClause := strings.Join(conditions, " AND ")

	var total int64
	countQuery := `SELECT COUNT(DISTINCT ep.order_id) FROM email_promotion ep WHERE ` + whereClause
	if err := r.db.QueryRowxContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count project email promotions: %w", err)
	}

	offset := (page - 1) * size
	query := `
		SELECT
			` + emailPromotionSelectColumns + `
		FROM email_promotion ep
		WHERE ` + whereClause + `
		ORDER BY ` + promotionPromotedAtExpr("ep") + ` DESC, ep.created_at DESC, ep.id DESC
		LIMIT ? OFFSET ?
	`
	dataArgs := append([]interface{}{}, args...)
	dataArgs = append(dataArgs, size, offset)

	var promotions []models.EmailPromotion
	if err := r.db.SelectContext(ctx, &promotions, query, dataArgs...); err != nil {
		return nil, 0, fmt.Errorf("query project email promotions: %w", err)
	}
	return promotions, total, nil
}

func promotionRealRank(alias string) string {
	return "CASE WHEN " + alias + ".channel = 'EMAIL' AND " + alias + ".business_tag = 'project_promotion' AND " + alias + ".trace_id IS NOT NULL AND " + alias + ".trace_id <> '' THEN 0 ELSE 1 END"
}

func promotionStatusRank(alias string) string {
	return "CASE " + alias + ".status WHEN 2 THEN 0 WHEN 1 THEN 1 WHEN 0 THEN 2 WHEN 3 THEN 3 ELSE 4 END"
}

func promotionPromotedAtExpr(alias string) string {
	return "COALESCE(" + alias + ".started_at, " + alias + ".created_at)"
}

func bestPromotionPredicate(alias, otherAlias string) string {
	return `NOT EXISTS (
		SELECT 1
		FROM email_promotion ` + otherAlias + `
		WHERE ` + otherAlias + `.project_id = ` + alias + `.project_id
		  AND ` + otherAlias + `.order_id = ` + alias + `.order_id
		  AND (
		    ` + promotionRealRank(otherAlias) + ` < ` + promotionRealRank(alias) + `
		    OR (` + promotionRealRank(otherAlias) + ` = ` + promotionRealRank(alias) + ` AND ` + promotionPromotedAtExpr(otherAlias) + ` > ` + promotionPromotedAtExpr(alias) + `)
		    OR (` + promotionRealRank(otherAlias) + ` = ` + promotionRealRank(alias) + ` AND ` + promotionPromotedAtExpr(otherAlias) + ` = ` + promotionPromotedAtExpr(alias) + ` AND ` + promotionStatusRank(otherAlias) + ` < ` + promotionStatusRank(alias) + `)
		    OR (` + promotionRealRank(otherAlias) + ` = ` + promotionRealRank(alias) + ` AND ` + promotionPromotedAtExpr(otherAlias) + ` = ` + promotionPromotedAtExpr(alias) + ` AND ` + promotionStatusRank(otherAlias) + ` = ` + promotionStatusRank(alias) + ` AND ` + otherAlias + `.created_at > ` + alias + `.created_at)
		    OR (` + promotionRealRank(otherAlias) + ` = ` + promotionRealRank(alias) + ` AND ` + promotionPromotedAtExpr(otherAlias) + ` = ` + promotionPromotedAtExpr(alias) + ` AND ` + promotionStatusRank(otherAlias) + ` = ` + promotionStatusRank(alias) + ` AND ` + otherAlias + `.created_at = ` + alias + `.created_at AND ` + otherAlias + `.id > ` + alias + `.id)
		  )
	)`
}

// ProjectPromotionUser is a safe display row for a promotion recipient.
type ProjectPromotionUser struct {
	UserID             int      `db:"user_id" json:"userId"`
	TalentProfileID    *int     `db:"talent_profile_id" json:"talentProfileId"`
	Nickname           *string  `db:"nickname" json:"nickname"`
	AvatarUrl          *string  `db:"avatar_url" json:"avatarUrl"`
	AuthStatus         int      `db:"auth_status" json:"authStatus"`
	CollaborationScore *float64 `db:"collaboration_score" json:"collaborationScore,omitempty"`
}

// ListProjectPromotionUsers returns recipient users for a promotion batch.
// It prefers email_promotion_recipient. Historical rows without recipient
// snapshots fall back to matching email_task.recipient_email to user.email, but
// contact fields are never selected into the response model.
func (r *EmailPromotionRepository) ListProjectPromotionUsers(ctx context.Context, batchID, page, size int) ([]ProjectPromotionUser, int64, error) {
	var recipientTotal int64
	if err := r.db.QueryRowxContext(ctx,
		`SELECT COUNT(*) FROM email_promotion_recipient WHERE promotion_id = ?`,
		batchID,
	).Scan(&recipientTotal); err != nil {
		return nil, 0, fmt.Errorf("count promotion recipients: %w", err)
	}

	offset := (page - 1) * size
	var users []ProjectPromotionUser
	if recipientTotal > 0 {
		query := `
			SELECT
				epr.user_id,
				tp.id AS talent_profile_id,
				u.nickname,
				u.avatar_url,
				COALESCE(u.auth_status, 0) AS auth_status,
				u.collaboration_score
			FROM email_promotion_recipient epr
			JOIN ` + "`user`" + ` u ON u.id = epr.user_id
			LEFT JOIN (
				SELECT user_id, MIN(id) AS id
				FROM talent_profile
				GROUP BY user_id
			) tp ON tp.user_id = epr.user_id
			WHERE epr.promotion_id = ?
			ORDER BY epr.created_at ASC, epr.id ASC
			LIMIT ? OFFSET ?
		`
		if err := r.db.SelectContext(ctx, &users, query, batchID, size, offset); err != nil {
			return nil, 0, fmt.Errorf("query promotion recipients: %w", err)
		}
		fillPromotionUserAvatarURLs(users)
		return users, recipientTotal, nil
	}

	var fallbackTotal int64
	if err := r.db.QueryRowxContext(ctx,
		`SELECT COUNT(DISTINCT u.id)
		 FROM email_task et
		 JOIN `+"`user`"+` u ON u.email = et.recipient_email
		 WHERE et.promotion_id = ?
		   AND et.recipient_email IS NOT NULL AND et.recipient_email != ''
		   AND u.email IS NOT NULL AND u.email != ''`,
		batchID,
	).Scan(&fallbackTotal); err != nil {
		return nil, 0, fmt.Errorf("count fallback promotion users: %w", err)
	}

	fallbackQuery := `
		SELECT
			u.id AS user_id,
			tp.id AS talent_profile_id,
			u.nickname,
			u.avatar_url,
			COALESCE(u.auth_status, 0) AS auth_status,
			u.collaboration_score
		FROM email_task et
		JOIN ` + "`user`" + ` u ON u.email = et.recipient_email
		LEFT JOIN (
			SELECT user_id, MIN(id) AS id
			FROM talent_profile
			GROUP BY user_id
		) tp ON tp.user_id = u.id
		WHERE et.promotion_id = ?
		  AND et.recipient_email IS NOT NULL AND et.recipient_email != ''
		  AND u.email IS NOT NULL AND u.email != ''
		GROUP BY u.id, tp.id, u.nickname, u.avatar_url, u.auth_status, u.collaboration_score
		ORDER BY MIN(et.create_time) ASC, u.id ASC
		LIMIT ? OFFSET ?
	`
	if err := r.db.SelectContext(ctx, &users, fallbackQuery, batchID, size, offset); err != nil {
		return nil, 0, fmt.Errorf("query fallback promotion users: %w", err)
	}
	fillPromotionUserAvatarURLs(users)
	return users, fallbackTotal, nil
}

func fillPromotionUserAvatarURLs(users []ProjectPromotionUser) {
	for i := range users {
		if users[i].AvatarUrl != nil && *users[i].AvatarUrl != "" {
			fullURL := oss.FullURL(*users[i].AvatarUrl)
			users[i].AvatarUrl = &fullURL
		}
	}
}
