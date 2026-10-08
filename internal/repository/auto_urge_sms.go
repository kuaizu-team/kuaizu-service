package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"sync/atomic"
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
type AutoUrgeClaim struct {
	UserID        int
	CycleID       int64
	Token         string
	RequestKey    string
	ReconcileOnly bool
}

var ErrAutoUrgeClaimLost = errors.New("automatic SMS claim lost")

type AutoUrgeRepository struct {
	db       *sqlx.DB
	tracking atomic.Bool
}

func NewAutoUrgeRepository(db *sqlx.DB) *AutoUrgeRepository { return &AutoUrgeRepository{db: db} }
func (r *AutoUrgeRepository) EnableTracking()               { r.tracking.Store(true) }
func (r *AutoUrgeRepository) TrackingEnabled() bool         { return r != nil && r.tracking.Load() }

// All historical unresolved records count. Reading a record is not processing it.
const autoUrgePendingCTE = `WITH reviewers AS (
 SELECT id AS project_id,creator_id AS user_id FROM project WHERE status<>4 AND deleted_at IS NULL
 UNION
 SELECT pm.project_id,pm.user_id FROM project_members pm
 JOIN project p ON p.id=pm.project_id WHERE p.status<>4 AND p.deleted_at IS NULL
), pending AS (
 SELECT ob.receiver_id AS user_id,ob.created_at AS pending_at FROM olive_branch_record ob
 JOIN project p ON p.id=ob.related_project_id WHERE ob.status=0 AND p.status<>4 AND p.deleted_at IS NULL
 UNION ALL
 SELECT r.user_id,pa.applied_at AS pending_at FROM project_application pa
 JOIN reviewers r ON r.project_id=pa.project_id WHERE pa.status=0
), eligible AS (
 SELECT user_id,COUNT(*) AS pending_count,MIN(pending_at) AS oldest_pending_at FROM pending GROUP BY user_id
 HAVING COUNT(*)>=3 OR MIN(pending_at)<=DATE_SUB(NOW(),INTERVAL 168 HOUR)
) `

func (r *AutoUrgeRepository) CheckSchema(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `SELECT user_id,state,claim_token,attempt_count,pending_count,oldest_pending_at,last_attempt_at,next_retry_at,sent_at,message_record_id,error_code,error_message,cycle_id,closed_at,request_key,dispatched_at FROM auto_urge_sms_state LIMIT 0`)
	return err
}
func (r *AutoUrgeRepository) Candidates(ctx context.Context, afterID, limit int) ([]AutoUrgeCandidate, error) {
	var rows []AutoUrgeCandidate
	err := r.db.SelectContext(ctx, &rows, autoUrgePendingCTE+`
 SELECT u.id AS user_id,COALESCE(NULLIF(TRIM(u.nickname),''),'快组儿') AS nickname,
 COALESCE(e.pending_count,0) AS pending_count,e.oldest_pending_at
 FROM `+"`user`"+` u LEFT JOIN eligible e ON e.user_id=u.id
 LEFT JOIN auto_urge_sms_state a ON a.user_id=u.id
 WHERE u.id>? AND (
 (e.user_id IS NOT NULL AND u.user_status<>1 AND NULLIF(TRIM(u.phone),'') IS NOT NULL
 AND (a.user_id IS NULL OR (a.state IN ('ready','failed') AND (a.next_retry_at IS NULL OR a.next_retry_at<=NOW()))
 OR (a.state='sent' AND a.closed_at IS NOT NULL)))
 OR (a.request_key IS NOT NULL AND
 ((a.state='unknown' AND (a.next_retry_at IS NULL OR a.next_retry_at<=NOW()))
 OR (a.state='claimed' AND a.last_attempt_at<=DATE_SUB(NOW(),INTERVAL 30 MINUTE)))))
 ORDER BY u.id LIMIT ?`, afterID, limit)
	return rows, err
}
func (r *AutoUrgeRepository) Claim(ctx context.Context, userID int, token string) (*AutoUrgeClaim, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO auto_urge_sms_state (user_id) VALUES (?) ON DUPLICATE KEY UPDATE user_id=user_id`, userID); err != nil {
		return nil, err
	}
	var row struct {
		State  string     `db:"state"`
		Cycle  int64      `db:"cycle_id"`
		Key    *string    `db:"request_key"`
		Closed *time.Time `db:"closed_at"`
		Due    bool       `db:"due"`
		Stale  bool       `db:"stale"`
	}
	err = tx.GetContext(ctx, &row, `SELECT state,cycle_id,request_key,closed_at,
 (next_retry_at IS NULL OR next_retry_at<=NOW()) AS due,
 COALESCE(last_attempt_at<=DATE_SUB(NOW(),INTERVAL 30 MINUTE),0) AS stale
 FROM auto_urge_sms_state WHERE user_id=? FOR UPDATE`, userID)
	if err != nil {
		return nil, err
	}
	recovering := row.Key != nil && *row.Key != "" && ((row.State == "unknown" && row.Due) || (row.State == "claimed" && row.Stale))
	normal := ((row.State == "ready" || row.State == "failed") && row.Due) || (row.State == "sent" && row.Closed != nil)
	if !recovering && !normal {
		return nil, nil
	}
	claim := &AutoUrgeClaim{UserID: userID, CycleID: row.Cycle, Token: token, ReconcileOnly: recovering}
	if row.Key != nil {
		claim.RequestKey = *row.Key
	}
	newCycle := !recovering && row.Closed != nil
	if newCycle {
		claim.CycleID++
		claim.RequestKey = ""
	}
	if claim.RequestKey == "" {
		claim.RequestKey = fmt.Sprintf("auto-urge:%d:%d", userID, claim.CycleID)
	}
	// Keep the previous key until its result is resolved, even if the backlog cleared.
	_, err = tx.ExecContext(ctx, `UPDATE auto_urge_sms_state SET cycle_id=?,request_key=?,state='claimed',claim_token=?,
 attempt_count=IF(?,1,attempt_count+1),last_attempt_at=NOW(),next_retry_at=NULL,
 closed_at=IF(?,NULL,closed_at),dispatched_at=IF(?,NULL,dispatched_at),
 sent_at=IF(?,NULL,sent_at),message_record_id=IF(?,NULL,message_record_id),
 error_code=NULL,error_message=NULL WHERE user_id=?`,
		claim.CycleID, claim.RequestKey, token, newCycle, newCycle, newCycle, newCycle, newCycle, userID)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return claim, nil
}
func (r *AutoUrgeRepository) Recheck(ctx context.Context, userID int) (*AutoUrgeCandidate, error) {
	var c AutoUrgeCandidate
	err := r.db.GetContext(ctx, &c, `WITH pending AS (
 SELECT ob.created_at AS pending_at FROM olive_branch_record ob JOIN project p ON p.id=ob.related_project_id
 WHERE ob.receiver_id=? AND ob.status=0 AND p.status<>4 AND p.deleted_at IS NULL
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
func (r *AutoUrgeRepository) Snapshot(ctx context.Context, c AutoUrgeCandidate, claim AutoUrgeClaim) error {
	res, err := r.db.ExecContext(ctx, `UPDATE auto_urge_sms_state SET pending_count=?,oldest_pending_at=?
 WHERE user_id=? AND state='claimed' AND claim_token=? AND cycle_id=? AND closed_at IS NULL`,
		c.PendingCount, c.OldestPendingAt, claim.UserID, claim.Token, claim.CycleID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 1 {
		return nil
	}
	var owns bool
	err = r.db.GetContext(ctx, &owns, `SELECT EXISTS(SELECT 1 FROM auto_urge_sms_state
 WHERE user_id=? AND state='claimed' AND claim_token=? AND cycle_id=? AND closed_at IS NULL)`, claim.UserID, claim.Token, claim.CycleID)
	if err != nil {
		return err
	}
	if !owns {
		return ErrAutoUrgeClaimLost
	}
	return nil
}
func (r *AutoUrgeRepository) MarkDispatched(ctx context.Context, claim AutoUrgeClaim) error {
	res, err := r.db.ExecContext(ctx, `UPDATE auto_urge_sms_state SET dispatched_at=NOW()
 WHERE user_id=? AND state='claimed' AND claim_token=? AND cycle_id=? AND request_key=? AND closed_at IS NULL`,
		claim.UserID, claim.Token, claim.CycleID, claim.RequestKey)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrAutoUrgeClaimLost
	}
	return nil
}
func (r *AutoUrgeRepository) Finish(ctx context.Context, claim AutoUrgeClaim, result AutoUrgeResult) error {
	switch result.State {
	case "ready", "sent", "failed", "unknown":
	default:
		return fmt.Errorf("invalid automatic SMS state %q", result.State)
	}
	res, err := r.db.ExecContext(ctx, `UPDATE auto_urge_sms_state SET state=?,message_record_id=?,error_code=?,error_message=?,
 next_retry_at=CASE WHEN ? IN ('ready','failed','unknown') THEN DATE_ADD(CURDATE(),INTERVAL 1 DAY) ELSE NULL END,
 sent_at=CASE WHEN ?='sent' THEN NOW() ELSE sent_at END
 WHERE user_id=? AND state='claimed' AND claim_token=? AND cycle_id=? AND request_key=?`,
		result.State, result.RecordID, result.ErrorCode, result.ErrorMessage, result.State, result.State, claim.UserID, claim.Token, claim.CycleID, claim.RequestKey)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrAutoUrgeClaimLost
	}
	return nil
}

// Called before adding work and after removing it, in the existing business
// transaction. Never resets an in-flight key or sends an external notification.
func (r *AutoUrgeRepository) ObserveClearanceTx(ctx context.Context, tx *sqlx.Tx, userIDs ...int) error {
	if !r.TrackingEnabled() {
		return nil
	}
	ids := append([]int(nil), userIDs...)
	sort.Ints(ids)
	unique := ids[:0]
	for _, id := range ids {
		if id > 0 && (len(unique) == 0 || unique[len(unique)-1] != id) {
			unique = append(unique, id)
		}
	}
	if len(unique) == 0 {
		return nil
	}
	query, args, err := sqlx.In(`UPDATE auto_urge_sms_state a SET closed_at=NOW()
 WHERE a.user_id IN (?) AND a.closed_at IS NULL
 AND NOT EXISTS(SELECT 1 FROM olive_branch_record ob JOIN project p ON p.id=ob.related_project_id
 WHERE ob.receiver_id=a.user_id AND ob.status=0 AND p.status<>4 AND p.deleted_at IS NULL)
 AND NOT EXISTS(SELECT 1 FROM project_application pa JOIN project p ON p.id=pa.project_id
 WHERE pa.status=0 AND p.status<>4 AND p.deleted_at IS NULL AND
 (p.creator_id=a.user_id OR EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=p.id AND pm.user_id=a.user_id)))`, unique)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(query), args...)
	return err
}
func (r *AutoUrgeRepository) ProjectRecipientsTx(ctx context.Context, tx *sqlx.Tx, projectID int) ([]int, error) {
	if !r.TrackingEnabled() {
		return nil, nil
	}
	var ids []int
	err := tx.SelectContext(ctx, &ids, `SELECT creator_id AS user_id FROM project WHERE id=?
 UNION SELECT user_id FROM project_members WHERE project_id=?
 UNION SELECT receiver_id FROM olive_branch_record WHERE related_project_id=?`, projectID, projectID, projectID)
	return ids, err
}
func (r *AutoUrgeRepository) ObserveProjectClearanceTx(ctx context.Context, tx *sqlx.Tx, projectID int, extraIDs ...int) error {
	ids, err := r.ProjectRecipientsTx(ctx, tx, projectID)
	if err != nil {
		return err
	}
	return r.ObserveClearanceTx(ctx, tx, append(ids, extraIDs...)...)
}
