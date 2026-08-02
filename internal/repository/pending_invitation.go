package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

// PendingInvitationRepository handles pending invitation display flags.
type PendingInvitationRepository struct {
	db *sqlx.DB
}

func NewPendingInvitationRepository(db *sqlx.DB) *PendingInvitationRepository {
	return &PendingInvitationRepository{db: db}
}

func (r *PendingInvitationRepository) Upsert(ctx context.Context, userID int, inviteType string, expireAt time.Time) error {
	query := `
		INSERT INTO pending_invitation (user_id, invite_type, expire_at)
		VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE
			expire_at = VALUES(expire_at),
			updated_at = CURRENT_TIMESTAMP
	`
	if _, err := r.db.ExecContext(ctx, query, userID, inviteType, expireAt); err != nil {
		return fmt.Errorf("upsert pending invitation: %w", err)
	}
	return nil
}

func (r *PendingInvitationRepository) GetActiveByUserID(ctx context.Context, userID int, now time.Time) (*models.PendingInvitation, error) {
	query := `
		SELECT id, user_id, invite_type, expire_at, created_at, updated_at
		FROM pending_invitation
		WHERE user_id = ? AND expire_at > ?
		ORDER BY updated_at DESC
		LIMIT 1
	`
	var item models.PendingInvitation
	if err := r.db.QueryRowxContext(ctx, query, userID, now).StructScan(&item); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query active pending invitation: %w", err)
	}
	return &item, nil
}

func (r *PendingInvitationRepository) ClearByUserID(ctx context.Context, userID int) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM pending_invitation WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("clear pending invitation: %w", err)
	}
	return nil
}

func (r *PendingInvitationRepository) DeleteExpired(ctx context.Context, now time.Time, limit int) (int64, error) {
	if limit <= 0 {
		return 0, nil
	}
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM pending_invitation WHERE expire_at <= ? ORDER BY expire_at LIMIT ?`,
		now, limit,
	)
	if err != nil {
		return 0, fmt.Errorf("delete expired pending invitations: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("get expired pending invitation delete count: %w", err)
	}
	return count, nil
}
