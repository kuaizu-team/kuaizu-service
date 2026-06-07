package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

// InvitationFeedbackRepository handles invitation feedback database operations.
type InvitationFeedbackRepository struct {
	db *sqlx.DB
}

func NewInvitationFeedbackRepository(db *sqlx.DB) *InvitationFeedbackRepository {
	return &InvitationFeedbackRepository{db: db}
}

func (r *InvitationFeedbackRepository) GetByUserID(ctx context.Context, userID int) (*models.InvitationFeedback, error) {
	query := `
		SELECT id, user_id, status, intention_text, conversation_status, created_at, updated_at
		FROM invitation_feedback
		WHERE user_id = ?
	`
	var f models.InvitationFeedback
	if err := r.db.QueryRowxContext(ctx, query, userID).StructScan(&f); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query invitation feedback by user id: %w", err)
	}
	return &f, nil
}

func (r *InvitationFeedbackRepository) UpsertFeedback(ctx context.Context, userID int, status string, intentionText *string) (*models.InvitationFeedback, error) {
	query := `
		INSERT INTO invitation_feedback (user_id, status, intention_text, conversation_status)
		VALUES (?, ?, ?, NULL)
		ON DUPLICATE KEY UPDATE
			status = VALUES(status),
			intention_text = VALUES(intention_text),
			conversation_status = NULL,
			updated_at = CURRENT_TIMESTAMP
	`
	if _, err := r.db.ExecContext(ctx, query, userID, status, intentionText); err != nil {
		return nil, fmt.Errorf("upsert invitation feedback: %w", err)
	}
	return r.GetByUserID(ctx, userID)
}

func (r *InvitationFeedbackRepository) ResetAfterInviteSent(ctx context.Context, userID int) (*models.InvitationFeedback, error) {
	query := `
		INSERT INTO invitation_feedback (user_id, status, intention_text, conversation_status)
		VALUES (?, ?, NULL, NULL)
		ON DUPLICATE KEY UPDATE
			status = VALUES(status),
			intention_text = NULL,
			conversation_status = NULL,
			updated_at = CURRENT_TIMESTAMP
	`
	if _, err := r.db.ExecContext(ctx, query, userID, models.InvitationFeedbackStatusPending); err != nil {
		return nil, fmt.Errorf("reset invitation feedback after invite sent: %w", err)
	}
	return r.GetByUserID(ctx, userID)
}

func (r *InvitationFeedbackRepository) UpsertConversationStatus(ctx context.Context, userID int, conversationStatus string) (*models.InvitationFeedback, error) {
	query := `
		INSERT INTO invitation_feedback (user_id, status, conversation_status)
		VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE
			conversation_status = VALUES(conversation_status),
			updated_at = CURRENT_TIMESTAMP
	`
	if _, err := r.db.ExecContext(ctx, query, userID, models.InvitationFeedbackStatusPending, conversationStatus); err != nil {
		return nil, fmt.Errorf("upsert invitation conversation status: %w", err)
	}
	return r.GetByUserID(ctx, userID)
}
