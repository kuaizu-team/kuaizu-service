package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

const smsNoticeSelectColumns = `
	id, channel, business_tag, trace_id, order_id,
	COALESCE(olive_branch_record_id, 0) AS olive_branch_record_id,
	member_removal_id, project_id, sender_id, receiver_id, sms_content, status, error_message,
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
		if isDuplicateEntryError(err) {
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

func (r *SmsNoticeRepository) CreateOutcome(ctx context.Context, notice *models.SmsNotice) error {
	query := `INSERT INTO olive_branch_sms_notice
		(channel,business_tag,trace_id,order_id,olive_branch_record_id,project_id,sender_id,receiver_id,sms_content,status,started_at)
		VALUES (:channel,:business_tag,:trace_id,:order_id,:olive_branch_record_id,:project_id,:sender_id,:receiver_id,:sms_content,:status,:started_at)`
	result, err := r.db.NamedExecContext(ctx, query, notice)
	if err != nil {
		if isDuplicateEntryError(err) {
			existing, getErr := r.GetByOrderID(ctx, notice.OrderID)
			if getErr != nil {
				return getErr
			}
			if existing != nil {
				*notice = *existing
				return nil
			}
		}
		return fmt.Errorf("create outcome sms notice: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get outcome sms notice id: %w", err)
	}
	notice.ID = int(id)
	return nil
}

func (r *SmsNoticeRepository) CreateMemberRemoval(ctx context.Context, notice *models.SmsNotice, removalID int64) error {
	result, err := r.db.ExecContext(ctx, `INSERT INTO olive_branch_sms_notice
		(channel,business_tag,trace_id,order_id,olive_branch_record_id,member_removal_id,project_id,sender_id,receiver_id,sms_content,status,started_at)
		VALUES (?,?,?,?,NULL,?,?,?,?,?,?,?)`, notice.Channel, notice.BusinessTag, notice.TraceID, notice.OrderID, removalID,
		notice.ProjectID, notice.SenderID, notice.ReceiverID, notice.SmsContent, notice.Status, notice.StartedAt)
	if err != nil {
		return fmt.Errorf("create member removal sms notice: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get member removal sms notice id: %w", err)
	}
	notice.ID = int(id)
	return nil
}

func (r *SmsNoticeRepository) CreateApplication(ctx context.Context, notice *models.SmsNotice) error {
	result, err := r.db.ExecContext(ctx, `INSERT INTO olive_branch_sms_notice
		(channel,business_tag,trace_id,order_id,olive_branch_record_id,member_removal_id,project_id,sender_id,receiver_id,sms_content,status,started_at)
		VALUES (?,?,?,?,NULL,NULL,?,?,?,?,?,?)`, notice.Channel, notice.BusinessTag, notice.TraceID, notice.OrderID,
		notice.ProjectID, notice.SenderID, notice.ReceiverID, notice.SmsContent, notice.Status, notice.StartedAt)
	if err != nil {
		if isDuplicateEntryError(err) {
			existing, getErr := r.GetByOrderID(ctx, notice.OrderID)
			if getErr != nil {
				return getErr
			}
			if existing != nil {
				*notice = *existing
				return nil
			}
		}
		return fmt.Errorf("create application sms notice: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get application sms notice id: %w", err)
	}
	notice.ID = int(id)
	return nil
}

func (r *SmsNoticeRepository) CompleteMemberRemoval(ctx context.Context, notice *models.SmsNotice) error {
	_, err := r.db.ExecContext(ctx, `UPDATE olive_branch_sms_notice SET status=?,error_message=?,completed_at=?,updated_at=NOW() WHERE id=?`, notice.Status, notice.ErrorMessage, notice.CompletedAt, notice.ID)
	if err != nil {
		return fmt.Errorf("complete member removal sms notice: %w", err)
	}
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

func (r *SmsNoticeRepository) MarkFailedAndOrderPushIfNotCompleted(
	ctx context.Context, noticeID, orderID int, message string, completedAt time.Time,
) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin failed sms notice transition: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	noticeResult, err := tx.ExecContext(ctx, `UPDATE olive_branch_sms_notice SET
		status=?, error_message=?, completed_at=?, updated_at=NOW()
		WHERE id=? AND (status IS NULL OR status<>?)`,
		models.SmsNoticeStatusFailed, message, completedAt, noticeID, models.SmsNoticeStatusCompleted)
	if err != nil {
		return false, fmt.Errorf("conditionally fail sms notice: %w", err)
	}
	noticeAffected, err := noticeResult.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read failed sms notice affected rows: %w", err)
	}
	if noticeAffected == 0 {
		return false, nil
	}

	orderResult, err := tx.ExecContext(ctx, `UPDATE `+"`order`"+` SET
		push_status='failed', push_error_message=?, last_push_time=?, updated_at=NOW()
		WHERE id=? AND (push_status IS NULL OR push_status<>'success')`,
		message, completedAt, orderID)
	if err != nil {
		return false, fmt.Errorf("conditionally fail order push: %w", err)
	}
	orderAffected, err := orderResult.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read failed order push affected rows: %w", err)
	}
	if orderAffected == 0 {
		var pushStatus sql.NullString
		if err := tx.QueryRowxContext(ctx,
			`SELECT push_status FROM `+"`order`"+` WHERE id=?`, orderID).Scan(&pushStatus); err != nil {
			return false, fmt.Errorf("read order push status after conditional failure: %w", err)
		}
		if pushStatus.Valid && pushStatus.String == "success" {
			return false, nil
		}
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit failed sms notice transition: %w", err)
	}
	return true, nil
}

func isDuplicateEntryError(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}

func (r *SmsNoticeRepository) GetByID(ctx context.Context, id int) (*models.SmsNotice, error) {
	return r.getOne(ctx, `SELECT `+smsNoticeSelectColumns+` FROM olive_branch_sms_notice WHERE id = ?`, id)
}

func (r *SmsNoticeRepository) GetByOliveBranchRecordID(ctx context.Context, oliveBranchRecordID int) (*models.SmsNotice, error) {
	return r.getOne(ctx, `SELECT `+smsNoticeSelectColumns+` FROM olive_branch_sms_notice WHERE olive_branch_record_id = ? AND business_tag = 'olive_branch_sms_notice' ORDER BY id DESC LIMIT 1`, oliveBranchRecordID)
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
