package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

const invitationRecordSelectColumns = `
	id, phone, project_id, inviter_user_id, role, status, provider, provider_biz_id,
	error_message, sent_at, registered_at, joined_at, created_at, updated_at
`

type InvitationRecordRepository struct {
	db *sqlx.DB
}

func NewInvitationRecordRepository(db *sqlx.DB) *InvitationRecordRepository {
	return &InvitationRecordRepository{db: db}
}

func (r *InvitationRecordRepository) GetByPhoneProject(ctx context.Context, phone string, projectID int) (*models.InvitationRecord, error) {
	return r.getOne(ctx, `SELECT `+invitationRecordSelectColumns+` FROM invitation_record WHERE phone = ? AND project_id = ? LIMIT 1`, phone, projectID)
}

func (r *InvitationRecordRepository) ListPendingJoinByPhone(ctx context.Context, phone string) ([]models.InvitationRecord, error) {
	var records []models.InvitationRecord
	query := `
		SELECT ` + invitationRecordSelectColumns + `
		FROM invitation_record
		WHERE phone = ?
		  AND status = ?
		  AND joined_at IS NULL
		ORDER BY id ASC
	`
	if err := r.db.SelectContext(ctx, &records, query, phone, models.InvitationRecordStatusSent); err != nil {
		return nil, fmt.Errorf("query pending invitation records: %w", err)
	}
	return records, nil
}

func (r *InvitationRecordRepository) Create(ctx context.Context, record *models.InvitationRecord) error {
	query := `
		INSERT INTO invitation_record (
			phone, project_id, inviter_user_id, role, status, provider, provider_biz_id,
			error_message, sent_at, registered_at, joined_at
		) VALUES (
			:phone, :project_id, :inviter_user_id, :role, :status, :provider, :provider_biz_id,
			:error_message, :sent_at, :registered_at, :joined_at
		)
	`
	result, err := r.db.NamedExecContext(ctx, query, record)
	if err != nil {
		if isDuplicateEntryError(err) {
			existing, getErr := r.GetByPhoneProject(ctx, record.Phone, record.ProjectID)
			if getErr != nil {
				return getErr
			}
			if existing != nil {
				*record = *existing
				return nil
			}
		}
		return fmt.Errorf("create invitation record: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	record.ID = int(id)
	return nil
}

func (r *InvitationRecordRepository) Update(ctx context.Context, record *models.InvitationRecord) error {
	query := `
		UPDATE invitation_record SET
			role = :role,
			status = :status,
			provider = :provider,
			provider_biz_id = :provider_biz_id,
			error_message = :error_message,
			sent_at = :sent_at,
			registered_at = :registered_at,
			joined_at = :joined_at,
			updated_at = NOW()
		WHERE id = :id
	`
	if _, err := r.db.NamedExecContext(ctx, query, record); err != nil {
		return fmt.Errorf("update invitation record: %w", err)
	}
	return nil
}

func (r *InvitationRecordRepository) MarkJoined(ctx context.Context, id int) error {
	query := `
		UPDATE invitation_record
		SET status = ?,
			registered_at = COALESCE(registered_at, NOW()),
			joined_at = COALESCE(joined_at, NOW()),
			error_message = NULL,
			updated_at = NOW()
		WHERE id = ?
	`
	if _, err := r.db.ExecContext(ctx, query, models.InvitationRecordStatusJoined, id); err != nil {
		return fmt.Errorf("mark invitation joined: %w", err)
	}
	return nil
}

func (r *InvitationRecordRepository) getOne(ctx context.Context, query string, args ...interface{}) (*models.InvitationRecord, error) {
	var record models.InvitationRecord
	if err := r.db.QueryRowxContext(ctx, query, args...).StructScan(&record); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query invitation record: %w", err)
	}
	return &record, nil
}
