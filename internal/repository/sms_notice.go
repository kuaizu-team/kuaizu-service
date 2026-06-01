package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

const smsNoticeSelectColumns = `
	id, channel, business_tag, trace_id, order_id, olive_branch_record_id,
	project_id, sender_id, receiver_id, sms_content, status, error_message,
	provider, provider_biz_id, started_at, completed_at, created_at, updated_at
`

type SmsNoticeRepository struct {
	db *sqlx.DB
}

func NewSmsNoticeRepository(db *sqlx.DB) *SmsNoticeRepository {
	return &SmsNoticeRepository{db: db}
}

func (r *SmsNoticeRepository) Create(ctx context.Context, notice *models.SmsNotice) error {
	query := `
		INSERT INTO olive_branch_sms_notice (
			channel, business_tag, trace_id, order_id, olive_branch_record_id,
			project_id, sender_id, receiver_id, sms_content, status, error_message,
			provider, provider_biz_id, started_at, completed_at
		) VALUES (
			:channel, :business_tag, :trace_id, :order_id, :olive_branch_record_id,
			:project_id, :sender_id, :receiver_id, :sms_content, :status, :error_message,
			:provider, :provider_biz_id, :started_at, :completed_at
		)
	`

	result, err := r.db.NamedExecContext(ctx, query, notice)
	if err != nil {
		existing, getErr := r.GetByOliveBranchRecordID(ctx, notice.OliveBranchRecordID)
		if getErr != nil {
			return getErr
		}
		if existing != nil {
			*notice = *existing
			return nil
		}
		existing, getErr = r.GetByOrderID(ctx, notice.OrderID)
		if getErr != nil {
			return getErr
		}
		if existing != nil {
			*notice = *existing
			return nil
		}
		return fmt.Errorf("create sms notice: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	notice.ID = int(id)
	return nil
}

func (r *SmsNoticeRepository) Update(ctx context.Context, notice *models.SmsNotice) error {
	query := `
		UPDATE olive_branch_sms_notice SET
			channel = :channel,
			business_tag = :business_tag,
			trace_id = :trace_id,
			order_id = :order_id,
			olive_branch_record_id = :olive_branch_record_id,
			project_id = :project_id,
			sender_id = :sender_id,
			receiver_id = :receiver_id,
			sms_content = :sms_content,
			status = :status,
			error_message = :error_message,
			provider = :provider,
			provider_biz_id = :provider_biz_id,
			started_at = :started_at,
			completed_at = :completed_at,
			updated_at = NOW()
		WHERE id = :id
	`
	if _, err := r.db.NamedExecContext(ctx, query, notice); err != nil {
		return fmt.Errorf("update sms notice: %w", err)
	}
	return nil
}

func (r *SmsNoticeRepository) GetByID(ctx context.Context, id int) (*models.SmsNotice, error) {
	return r.getOne(ctx, `SELECT `+smsNoticeSelectColumns+` FROM olive_branch_sms_notice WHERE id = ?`, id)
}

func (r *SmsNoticeRepository) GetByOliveBranchRecordID(ctx context.Context, oliveBranchRecordID int) (*models.SmsNotice, error) {
	return r.getOne(ctx, `SELECT `+smsNoticeSelectColumns+` FROM olive_branch_sms_notice WHERE olive_branch_record_id = ?`, oliveBranchRecordID)
}

func (r *SmsNoticeRepository) GetByOrderID(ctx context.Context, orderID int) (*models.SmsNotice, error) {
	return r.getOne(ctx, `SELECT `+smsNoticeSelectColumns+` FROM olive_branch_sms_notice WHERE order_id = ? ORDER BY created_at DESC, id DESC LIMIT 1`, orderID)
}

func (r *SmsNoticeRepository) getOne(ctx context.Context, query string, args ...interface{}) (*models.SmsNotice, error) {
	var notice models.SmsNotice
	if err := r.db.QueryRowxContext(ctx, query, args...).StructScan(&notice); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query sms notice: %w", err)
	}
	return &notice, nil
}
