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
	Page   int
	Size   int
	Status *int
	UserID *int
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

	whereClause := strings.Join(conditions, " AND ")

	// Count total
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM feedback f WHERE %s`, whereClause)
	var total int64
	if err := r.db.QueryRowxContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count feedbacks: %w", err)
	}

	// Query with pagination
	offset := (params.Page - 1) * params.Size
	query := fmt.Sprintf(`
		SELECT
			f.id, f.user_id, f.content, f.contact_image,
			f.status, f.admin_reply, f.created_at, f.updated_at,
			u.nickname
		FROM feedback f
		LEFT JOIN `+"`user`"+` u ON f.user_id = u.id
		WHERE %s
		ORDER BY f.created_at DESC
		LIMIT ? OFFSET ?
	`, whereClause)
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
			f.id, f.user_id, f.content, f.contact_image,
			f.status, f.admin_reply, f.created_at, f.updated_at,
			u.nickname
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

// Reply sets admin reply and marks feedback as handled
func (r *FeedbackRepository) Reply(ctx context.Context, id int, reply string) error {
	query := `UPDATE feedback SET admin_reply = ?, status = 1, updated_at = CURRENT_TIMESTAMP WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query, reply, id)
	if err != nil {
		return fmt.Errorf("reply feedback: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("feedback not found")
	}

	return nil
}
