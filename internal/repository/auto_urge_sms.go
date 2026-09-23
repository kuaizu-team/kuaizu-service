package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

type AutoUrgeCandidate struct {
	UserID          int        `db:"user_id"`
	Nickname        string     `db:"nickname"`
	PendingCount    int        `db:"pending_count"`
	OldestPendingAt *time.Time `db:"oldest_pending_at"`
}

type AutoUrgeResult struct {
	State        string
	RecordID     *int64
	ErrorCode    string
	ErrorMessage string
}

type AutoUrgeRepository struct{ db *sqlx.DB }

func NewAutoUrgeRepository(db *sqlx.DB) *AutoUrgeRepository { return &AutoUrgeRepository{db: db} }

// UNION deduplicates creators who also have a membership, not distinct pending
// records. Read flags do not determine whether a record is still actionable.
const autoUrgePendingCTE = `WITH reviewers AS (
 SELECT id AS project_id, creator_id AS user_id FROM project WHERE status <> 4 AND deleted_at IS NULL
 UNION
 SELECT pm.project_id,pm.user_id FROM project_members pm
 JOIN project p ON p.id=pm.project_id WHERE p.status <> 4 AND p.deleted_at IS NULL
), pending AS (
 SELECT receiver_id AS user_id,created_at AS pending_at FROM olive_branch_record WHERE status=0
 UNION ALL
 SELECT r.user_id,pa.applied_at AS pending_at FROM project_application pa
 JOIN reviewers r ON r.project_id=pa.project_id WHERE pa.status=0
), eligible AS (
 SELECT user_id,COUNT(*) AS pending_count,MIN(pending_at) AS oldest_pending_at
 FROM pending GROUP BY user_id
 HAVING COUNT(*)>=3 OR MIN(pending_at)<=DATE_SUB(NOW(),INTERVAL 168 HOUR)
) `

func (r *AutoUrgeRepository) CheckSchema(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `SELECT user_id,state,claim_token,attempt_count,pending_count,oldest_pending_at,last_attempt_at,next_retry_at,sent_at,message_record_id,error_code,error_message FROM auto_urge_sms_state LIMIT 0`)
	return err
}

func (r *AutoUrgeRepository) Candidates(ctx context.Context, afterID, limit int) ([]AutoUrgeCandidate, error) {
	var rows []AutoUrgeCandidate
	err := r.db.SelectContext(ctx, &rows, autoUrgePendingCTE+`
 SELECT e.user_id,COALESCE(NULLIF(TRIM(u.nickname),''),'快组儿') AS nickname,e.pending_count,e.oldest_pending_at
 FROM eligible e JOIN `+"`user`"+` u ON u.id=e.user_id
 LEFT JOIN auto_urge_sms_state a ON a.user_id=e.user_id
 WHERE e.user_id>? AND u.user_status<>1 AND NULLIF(TRIM(u.phone),'') IS NOT NULL
 AND (a.user_id IS NULL OR (a.state IN ('ready','failed') AND (a.next_retry_at IS NULL OR a.next_retry_at<=NOW())))
 ORDER BY e.user_id LIMIT ?`, afterID, limit)
	return rows, err
}

func (r *AutoUrgeRepository) Claim(ctx context.Context, userID int, token string) (bool, error) {
	// Insert a harmless placeholder, then atomically compete for the claim. A crash
	// after claiming stays blocked and is reviewed manually, never lease-retried.
	if _, err := r.db.ExecContext(ctx, `INSERT INTO auto_urge_sms_state (user_id) VALUES (?) ON DUPLICATE KEY UPDATE user_id=user_id`, userID); err != nil {
		return false, err
	}
	result, err := r.db.ExecContext(ctx, `UPDATE auto_urge_sms_state SET state='claimed',claim_token=?,attempt_count=attempt_count+1,last_attempt_at=NOW(),next_retry_at=NULL,error_code=NULL,error_message=NULL
 WHERE user_id=? AND state IN ('ready','failed') AND (next_retry_at IS NULL OR next_retry_at<=NOW())`, token, userID)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

func (r *AutoUrgeRepository) Recheck(ctx context.Context, userID int) (*AutoUrgeCandidate, error) {
	var c AutoUrgeCandidate
	// Restrict the recheck to one recipient; do not regroup every user per send.
	err := r.db.GetContext(ctx, &c, `WITH pending AS (
 SELECT created_at AS pending_at FROM olive_branch_record WHERE receiver_id=? AND status=0
 UNION ALL
 SELECT pa.applied_at FROM project_application pa JOIN project p ON p.id=pa.project_id
 WHERE pa.status=0 AND p.status<>4 AND p.deleted_at IS NULL AND (p.creator_id=? OR EXISTS (
 SELECT 1 FROM project_members pm WHERE pm.project_id=p.id AND pm.user_id=?))
), eligible AS (SELECT COUNT(*) AS pending_count,MIN(pending_at) AS oldest_pending_at FROM pending
 HAVING COUNT(*)>=3 OR MIN(pending_at)<=DATE_SUB(NOW(),INTERVAL 168 HOUR))
SELECT u.id AS user_id,COALESCE(NULLIF(TRIM(u.nickname),''),'快组儿') AS nickname,e.pending_count,e.oldest_pending_at
FROM eligible e JOIN `+"`user`"+` u ON u.id=? WHERE u.user_status<>1 AND NULLIF(TRIM(u.phone),'') IS NOT NULL`, userID, userID, userID, userID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &c, err
}
func (r *AutoUrgeRepository) Snapshot(ctx context.Context, c AutoUrgeCandidate, token string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE auto_urge_sms_state SET pending_count=?,oldest_pending_at=? WHERE user_id=? AND state='claimed' AND claim_token=?`, c.PendingCount, c.OldestPendingAt, c.UserID, token)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		var owns bool
		if err := r.db.GetContext(ctx, &owns, `SELECT EXISTS(SELECT 1 FROM auto_urge_sms_state WHERE user_id=? AND state='claimed' AND claim_token=?)`, c.UserID, token); err != nil {
			return err
		}
		if owns {
			return nil
		}
	}
	if n != 1 {
		return fmt.Errorf("automatic SMS claim lost for user %d", c.UserID)
	}
	return nil
}

func (r *AutoUrgeRepository) Finish(ctx context.Context, userID int, token string, result AutoUrgeResult) error {
	switch result.State {
	case "ready", "sent", "failed", "unknown":
	default:
		return fmt.Errorf("invalid automatic SMS state %q", result.State)
	}
	// Delay even definite failures until the next daily scan. Manual SMS state is
	// deliberately untouched; uncertain outcomes and sent rows cannot be claimed.
	res, err := r.db.ExecContext(ctx, `UPDATE auto_urge_sms_state SET state=?,message_record_id=?,error_code=?,error_message=?,
 next_retry_at=CASE WHEN ? IN ('ready','failed') THEN DATE_ADD(CURDATE(),INTERVAL 1 DAY) ELSE NULL END,
 sent_at=CASE WHEN ?='sent' THEN NOW() ELSE sent_at END
 WHERE user_id=? AND state='claimed' AND claim_token=?`, result.State, result.RecordID, result.ErrorCode, result.ErrorMessage, result.State, result.State, userID, token)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("automatic SMS claim lost for user %d", userID)
	}
	return nil
}
