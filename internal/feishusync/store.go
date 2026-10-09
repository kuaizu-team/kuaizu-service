package feishusync

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"time"
)

type Job struct {
	ID        string    `db:"id" json:"id"`
	SchoolID  int       `db:"school_id" json:"schoolId"`
	AdminID   int       `db:"admin_id" json:"-"`
	Status    string    `db:"status" json:"status"`
	Total     int       `db:"total" json:"total"`
	Processed int       `db:"processed" json:"processed"`
	Created   int       `db:"created_count" json:"created"`
	Updated   int       `db:"updated_count" json:"updated"`
	Deleted   int       `db:"deleted_count" json:"deleted"`
	Message   string    `db:"message" json:"message"`
	CreatedAt time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt time.Time `db:"updated_at" json:"updatedAt"`
	URL       string    `json:"url,omitempty"`
}

const jobColumns = "id,school_id,admin_id,status,total,processed,created_count,updated_count,deleted_count,message,created_at,updated_at"
const targetColumns = "school_id,app_token,node_token,table_id,view_id,node_started,table_started"

type Record struct {
	UserID      int    `db:"user_id"`
	RecordID    string `db:"record_id"`
	ClientToken string `db:"client_token"`
	State       string `db:"state"`
}
type Store struct{ db *sqlx.DB }

func NewStore(db *sqlx.DB) *Store { return &Store{db: db} }

func (s *Store) CheckSchema(ctx context.Context) error {
	for _, query := range []string{
		"SELECT " + targetColumns + " FROM feishu_user_sync_target LIMIT 0",
		"SELECT " + jobColumns + ",active_school_id FROM feishu_user_sync_job LIMIT 0",
		"SELECT school_id,user_id,record_id,client_token,state,payload FROM feishu_user_sync_record LIMIT 0",
	} {
		rows, err := s.db.QueryContext(ctx, query)
		if err != nil {
			return fmt.Errorf("飞书同步表未就绪，请先执行 sql/20261008_feishu_user_sync.sql")
		}
		rows.Close()
	}
	return nil
}

// Lock the school row and unique active_school_id, so duplicate clicks reuse one job.
func (s *Store) Enqueue(ctx context.Context, adminID, schoolID int) (*Job, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "INSERT IGNORE INTO feishu_user_sync_target (school_id) VALUES (?)", schoolID); err != nil {
		return nil, err
	}
	var locked int
	if err = tx.GetContext(ctx, &locked, "SELECT school_id FROM feishu_user_sync_target WHERE school_id=? FOR UPDATE", schoolID); err != nil {
		return nil, err
	}
	var job Job
	err = tx.GetContext(ctx, &job, "SELECT "+jobColumns+" FROM feishu_user_sync_job WHERE active_school_id=?", schoolID)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if err == sql.ErrNoRows {
		job.ID = uuid.NewString()
		if _, err = tx.ExecContext(ctx, "INSERT INTO feishu_user_sync_job (id,school_id,admin_id,active_school_id,status,message) VALUES (?,?,?,?,'queued','等待同步')", job.ID, schoolID, adminID, schoolID); err != nil {
			return nil, err
		}
		if err = tx.GetContext(ctx, &job, "SELECT "+jobColumns+" FROM feishu_user_sync_job WHERE id=?", job.ID); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &job, nil
}
func (s *Store) Latest(ctx context.Context, schoolID int) (*Job, error) {
	var job Job
	err := s.db.GetContext(ctx, &job, "SELECT "+jobColumns+" FROM feishu_user_sync_job WHERE school_id=? ORDER BY created_at DESC,id DESC LIMIT 1", schoolID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &job, nil
}
func (s *Store) Next(ctx context.Context) (*Job, error) {
	var job Job
	err := s.db.GetContext(ctx, &job, "SELECT "+jobColumns+" FROM feishu_user_sync_job WHERE active_school_id IS NOT NULL ORDER BY created_at,id LIMIT 1")
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &job, nil
}
func (s *Store) Progress(ctx context.Context, job *Job) error {
	_, err := s.db.ExecContext(ctx, "UPDATE feishu_user_sync_job SET status=?,total=?,processed=?,created_count=?,updated_count=?,deleted_count=?,message=? WHERE id=?", job.Status, job.Total, job.Processed, job.Created, job.Updated, job.Deleted, job.Message, job.ID)
	return err
}
func (s *Store) Finish(ctx context.Context, job *Job) error {
	_, err := s.db.ExecContext(ctx, "UPDATE feishu_user_sync_job SET status=?,active_school_id=NULL,total=?,processed=?,created_count=?,updated_count=?,deleted_count=?,message=? WHERE id=?", job.Status, job.Total, job.Processed, job.Created, job.Updated, job.Deleted, job.Message, job.ID)
	return err
}
func (s *Store) Target(ctx context.Context, schoolID int) (Target, error) {
	var target Target
	err := s.db.GetContext(ctx, &target, "SELECT "+targetColumns+" FROM feishu_user_sync_target WHERE school_id=?", schoolID)
	return target, err
}
func (s *Store) SaveTarget(ctx context.Context, target Target) error {
	_, err := s.db.ExecContext(ctx, "UPDATE feishu_user_sync_target SET app_token=?,node_token=?,table_id=?,view_id=?,node_started=?,table_started=? WHERE school_id=?", target.AppToken, target.NodeToken, target.TableID, target.ViewID, target.NodeStarted, target.TableStarted, target.SchoolID)
	return err
}

func (s *Store) UserIDs(ctx context.Context, schoolID int) ([]int, error) {
	if schoolID <= 0 {
		return nil, fmt.Errorf("必须指定有效学校")
	}
	// Reuse the original export scope SQL; list filters do not restrict school sync.
	where, args, err := repository.UserFilterSQL(repository.UserListParams{SchoolID: &schoolID})
	if err != nil {
		return nil, err
	}
	var ids []int
	err = s.db.SelectContext(ctx, &ids, "SELECT u.id FROM `user` u WHERE "+where+" ORDER BY u.id", args...)
	return ids, err
}
func (s *Store) User(ctx context.Context, schoolID, userID int) (*UserRow, error) {
	// Same LEFT JOINs and field expressions as the removed CSV export. Reapply scope
	// immediately before each remote write, excluding deleted/transferred users.
	const query = "SELECT u.id,COALESCE(u.nickname,'') nickname,COALESCE(tp.mbti,'') mbti,COALESCE(s.school_name,'') school,COALESCE(m.major_name,'') major,COALESCE(CAST(u.grade AS CHAR),'') grade,COALESCE(tp.self_evaluation,'') intro,COALESCE(tp.project_experience,'') experience,u.collaboration_score score,COALESCE(u.auth_status,0) auth,COALESCE(tp.status,0) talent,COALESCE(u.phone,'') phone,COALESCE(u.wechat_id,'') wechat,COALESCE(u.email,'') email,u.user_status status FROM `user` u LEFT JOIN talent_profile tp ON tp.user_id=u.id LEFT JOIN school s ON s.id=u.school_id LEFT JOIN major m ON m.id=u.major_id WHERE u.school_id=? AND u.id=?"
	var row UserRow
	err := s.db.GetContext(ctx, &row, query, schoolID, userID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}
func (s *Store) Record(ctx context.Context, schoolID, userID int) (*Record, error) {
	var record Record
	err := s.db.GetContext(ctx, &record, "SELECT user_id,record_id,client_token,state FROM feishu_user_sync_record WHERE school_id=? AND user_id=?", schoolID, userID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}
func (s *Store) PrepareCreate(ctx context.Context, schoolID, userID int, token, payload string) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO feishu_user_sync_record (school_id,user_id,client_token,state,payload) VALUES (?,?,?,'pending',?)", schoolID, userID, token, payload)
	return err
}
func (s *Store) ConfirmCreate(ctx context.Context, schoolID, userID int, id string) error {
	result, err := s.db.ExecContext(ctx, "UPDATE feishu_user_sync_record SET record_id=?,state='ready',payload=NULL WHERE school_id=? AND user_id=? AND state='pending'", id, schoolID, userID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return fmt.Errorf("新增记录映射状态已变更，必须人工核对")
	}
	return nil
}
func (s *Store) Forget(ctx context.Context, schoolID, userID int) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM feishu_user_sync_record WHERE school_id=? AND user_id=?", schoolID, userID)
	return err
}
func (s *Store) Stale(ctx context.Context, schoolID int) ([]Record, error) {
	var records []Record
	err := s.db.SelectContext(ctx, &records, "SELECT r.user_id,r.record_id,r.client_token,r.state FROM feishu_user_sync_record r LEFT JOIN `user` u ON u.id=r.user_id AND u.school_id=r.school_id WHERE r.school_id=? AND u.id IS NULL", schoolID)
	return records, err
}
func (s *Store) Belongs(ctx context.Context, schoolID, userID int) (bool, error) {
	var count int
	err := s.db.GetContext(ctx, &count, "SELECT COUNT(*) FROM `user` WHERE id=? AND school_id=?", userID, schoolID)
	return count == 1, err
}
