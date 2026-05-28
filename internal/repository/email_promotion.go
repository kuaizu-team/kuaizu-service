package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/oss"
)

// EmailPromotionRepository handles email promotion database operations
type EmailPromotionRepository struct {
	db *sqlx.DB
}

// NewEmailPromotionRepository creates a new EmailPromotionRepository
func NewEmailPromotionRepository(db *sqlx.DB) *EmailPromotionRepository {
	return &EmailPromotionRepository{db: db}
}

// Create creates a new email promotion record
func (r *EmailPromotionRepository) Create(ctx context.Context, promotion *models.EmailPromotion) error {
	query := `
		INSERT INTO email_promotion (
			order_id, project_id, creator_id, max_recipients, total_sent, status
		) VALUES (
			:order_id, :project_id, :creator_id, :max_recipients, :total_sent, :status
		)
	`

	result, err := r.db.NamedExecContext(ctx, query, promotion)
	if err != nil {
		return fmt.Errorf("create email promotion: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}

	promotion.ID = int(id)
	return nil
}

// GetByID retrieves an email promotion by ID
func (r *EmailPromotionRepository) GetByID(ctx context.Context, id int) (*models.EmailPromotion, error) {
	query := `
		SELECT 
			ep.id, ep.order_id, ep.project_id, ep.creator_id,
			ep.max_recipients, ep.total_sent, ep.status,
			ep.error_message, ep.started_at, ep.completed_at, ep.created_at,
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
			id, order_id, project_id, creator_id,
			max_recipients, total_sent, status,
			error_message, started_at, completed_at, created_at
		FROM email_promotion
		WHERE order_id = ?
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

// Update updates an email promotion record
func (r *EmailPromotionRepository) Update(ctx context.Context, promotion *models.EmailPromotion) error {
	query := `
		UPDATE email_promotion SET
			total_sent = :total_sent,
			status = :status,
			error_message = :error_message,
			started_at = :started_at,
			completed_at = :completed_at
		WHERE id = :id
	`

	_, err := r.db.NamedExecContext(ctx, query, promotion)
	if err != nil {
		return fmt.Errorf("update email promotion: %w", err)
	}

	return nil
}

// ListByCreatorID retrieves email promotions by creator ID with pagination
func (r *EmailPromotionRepository) ListByCreatorID(ctx context.Context, creatorID int, page, size int) ([]models.EmailPromotion, int64, error) {
	// Count total
	countQuery := `SELECT COUNT(*) FROM email_promotion WHERE creator_id = ?`
	var total int64
	if err := r.db.QueryRowxContext(ctx, countQuery, creatorID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count email promotions: %w", err)
	}

	// Query with pagination
	offset := (page - 1) * size
	query := `
		SELECT 
			ep.id, ep.order_id, ep.project_id, ep.creator_id,
			ep.max_recipients, ep.total_sent, ep.status,
			ep.error_message, ep.started_at, ep.completed_at, ep.created_at,
			p.name AS project_name
		FROM email_promotion ep
		LEFT JOIN project p ON ep.project_id = p.id
		WHERE ep.creator_id = ?
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
			id, order_id, project_id, creator_id,
			max_recipients, total_sent, status,
			error_message, started_at, completed_at, created_at
		FROM email_promotion
		WHERE project_id = ?
		ORDER BY created_at DESC
	`

	var promotions []models.EmailPromotion
	if err := r.db.SelectContext(ctx, &promotions, query, projectID); err != nil {
		return nil, fmt.Errorf("query email promotions by project: %w", err)
	}

	return promotions, nil
}

// ListByProjectSince retrieves recent email promotions for a project.
func (r *EmailPromotionRepository) ListByProjectSince(ctx context.Context, projectID, days, limit int) ([]models.EmailPromotion, int64, error) {
	countQuery := `
		SELECT COUNT(*)
		FROM email_promotion
		WHERE project_id = ? AND created_at >= NOW() - INTERVAL ? DAY
	`
	var total int64
	if err := r.db.QueryRowxContext(ctx, countQuery, projectID, days).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count project email promotions: %w", err)
	}

	query := `
		SELECT
			id, order_id, project_id, creator_id,
			max_recipients, total_sent, status,
			error_message, started_at, completed_at, created_at
		FROM email_promotion
		WHERE project_id = ? AND created_at >= NOW() - INTERVAL ? DAY
		ORDER BY created_at DESC
		LIMIT ?
	`
	var promotions []models.EmailPromotion
	if err := r.db.SelectContext(ctx, &promotions, query, projectID, days, limit); err != nil {
		return nil, 0, fmt.Errorf("query project email promotions: %w", err)
	}

	return promotions, total, nil
}

// ProjectPromotionUser is a safe display row for a promotion recipient.
type ProjectPromotionUser struct {
	UserID          int     `db:"user_id" json:"userId"`
	TalentProfileID *int    `db:"talent_profile_id" json:"talentProfileId"`
	Nickname        *string `db:"nickname" json:"nickname"`
	AvatarUrl       *string `db:"avatar_url" json:"avatarUrl"`
	AuthStatus      int     `db:"auth_status" json:"authStatus"`
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
				COALESCE(u.auth_status, 0) AS auth_status
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
			COALESCE(u.auth_status, 0) AS auth_status
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
		GROUP BY u.id, tp.id, u.nickname, u.avatar_url, u.auth_status
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
