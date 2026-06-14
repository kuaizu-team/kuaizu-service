package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

// FeedbackRepository handles feedback database operations
type FeedbackRepository struct {
	db *sqlx.DB
}

// NewFeedbackRepository creates a new FeedbackRepository
func NewFeedbackRepository(db *sqlx.DB) *FeedbackRepository {
	return &FeedbackRepository{db: db}
}

// FeedbackListParams contains parameters for listing feedbacks
type FeedbackListParams struct {
	Page     int
	Size     int
	Status   *int
	UserID   *int
	SchoolID *int // admin 权限过滤：只返回该学校用户的反馈
}

// List retrieves paginated feedbacks with optional filters
func (r *FeedbackRepository) List(ctx context.Context, params FeedbackListParams) ([]models.Feedback, int64, error) {
	conditions := []string{"1=1"}
	args := []interface{}{}

	if params.Status != nil {
		conditions = append(conditions, "f.status = ?")
		args = append(args, *params.Status)
	}

	if params.UserID != nil {
		conditions = append(conditions, "f.user_id = ?")
		args = append(args, *params.UserID)
	}
	if params.SchoolID != nil {
		conditions = append(conditions, "u.school_id = ?")
		args = append(args, *params.SchoolID)
	}

	whereClause := strings.Join(conditions, " AND ")

	// Both count and data queries share the same JOIN so that u.school_id
	// in the WHERE clause is always valid regardless of which filters are set.
	const fromJoin = "FROM feedback f LEFT JOIN `user` u ON f.user_id = u.id"

	// Count total (must use the same JOIN as the data query)
	countQuery := fmt.Sprintf("SELECT COUNT(*) %s WHERE %s", fromJoin, whereClause)
	var total int64
	if err := r.db.QueryRowxContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count feedbacks: %w", err)
	}

	// Query with pagination
	offset := (params.Page - 1) * params.Size
	query := fmt.Sprintf(`
		SELECT
			f.id, f.user_id, f.content,
			f.email, f.need_receipt, f.status, f.admin_reply, f.created_at, f.updated_at,
			u.nickname, u.school_id AS user_school_id
		%s
		WHERE %s
		ORDER BY f.created_at DESC
		LIMIT ? OFFSET ?
	`, fromJoin, whereClause)
	args = append(args, params.Size, offset)

	var feedbacks []models.Feedback
	if err := r.db.SelectContext(ctx, &feedbacks, query, args...); err != nil {
		return nil, 0, fmt.Errorf("query feedbacks: %w", err)
	}

	return feedbacks, total, nil
}

// GetByID retrieves a feedback by ID
func (r *FeedbackRepository) GetByID(ctx context.Context, id int) (*models.Feedback, error) {
	query := `
		SELECT
			f.id, f.user_id, f.content,
			f.email, f.need_receipt, f.status, f.admin_reply, f.created_at, f.updated_at,
			u.nickname, u.school_id AS user_school_id
		FROM feedback f
		LEFT JOIN ` + "`user`" + ` u ON f.user_id = u.id
		WHERE f.id = ?
	`

	var f models.Feedback
	if err := r.db.QueryRowxContext(ctx, query, id).StructScan(&f); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query feedback by id: %w", err)
	}

	return &f, nil
}

// Create inserts a new feedback record submitted by a user.
func (r *FeedbackRepository) Create(ctx context.Context, f *models.Feedback) error {
	query := `
		INSERT INTO feedback (user_id, content, email, need_receipt, status)
		VALUES (:user_id, :content, :email, :need_receipt, :status)
	`
	result, err := r.db.NamedExecContext(ctx, query, f)
	if err != nil {
		return fmt.Errorf("create feedback: %w", err)
	}
	id, _ := result.LastInsertId()
	f.ID = int(id)
	return nil
}

// Reply updates admin_reply and optionally status.
// When status is nil only admin_reply and updated_at are touched.
func (r *FeedbackRepository) Reply(ctx context.Context, id int, reply string, status *int) error {
	var (
		query string
		args  []interface{}
	)
	if status != nil {
		query = `UPDATE feedback SET admin_reply = ?, status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
		args = []interface{}{reply, *status, id}
	} else {
		query = `UPDATE feedback SET admin_reply = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
		args = []interface{}{reply, id}
	}

	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("reply feedback: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("feedback not found")
	}

	return nil
}
