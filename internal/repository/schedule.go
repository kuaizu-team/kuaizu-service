package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

type ScheduleRepository struct{ db *sqlx.DB }

// Import is atomic. A nullable import_key leaves ordinary manually-created
// schedules unchanged; the unique key also deduplicates concurrent retries.
func (r *ScheduleRepository) Import(ctx context.Context, userID int, items []models.Schedule) (int, int, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	created, skipped := 0, 0
	for _, item := range items {
		item.ID = 0
		payload, err := json.Marshal(item)
		if err != nil {
			return 0, 0, err
		}
		hash := sha256.Sum256(payload)
		key := hex.EncodeToString(hash[:])
		_, err = tx.ExecContext(ctx, `INSERT INTO user_schedule (user_id,title,schedule_date,all_day,schedule_time,location,notes,import_key) VALUES (?,?,?,?,?,?,?,?)`, userID, item.Title, item.Date, item.AllDay, item.Time, item.Location, item.Notes, key)
		var duplicate *mysql.MySQLError
		if errors.As(err, &duplicate) && duplicate.Number == 1062 {
			skipped++
			continue
		}
		if err != nil {
			return 0, 0, err
		}
		created++
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return created, skipped, nil
}

func NewScheduleRepository(db *sqlx.DB) *ScheduleRepository { return &ScheduleRepository{db: db} }

func (r *ScheduleRepository) List(ctx context.Context, userID int, from, to string) ([]models.Schedule, error) {
	items := make([]models.Schedule, 0)
	err := r.db.SelectContext(ctx, &items, `SELECT id,title,DATE_FORMAT(schedule_date,'%Y-%m-%d') AS date,all_day,TIME_FORMAT(schedule_time,'%H:%i') AS time,location,notes FROM user_schedule WHERE user_id=? AND schedule_date>=? AND schedule_date<=? ORDER BY schedule_date,all_day DESC,schedule_time,id LIMIT 2001`, userID, from, to)
	return items, err
}
func (r *ScheduleRepository) Create(ctx context.Context, userID int, s *models.Schedule) error {
	res, err := r.db.ExecContext(ctx, `INSERT INTO user_schedule (user_id,title,schedule_date,all_day,schedule_time,location,notes) VALUES (?,?,?,?,?,?,?)`, userID, s.Title, s.Date, s.AllDay, s.Time, s.Location, s.Notes)
	if err != nil {
		return err
	}
	s.ID, err = res.LastInsertId()
	return err
}
func (r *ScheduleRepository) Update(ctx context.Context, userID int, s *models.Schedule) error {
	// updated_at also changes for real changes. An identical PUT must still succeed.
	res, err := r.db.ExecContext(ctx, `UPDATE user_schedule SET title=?,schedule_date=?,all_day=?,schedule_time=?,location=?,notes=? WHERE id=? AND user_id=?`, s.Title, s.Date, s.AllDay, s.Time, s.Location, s.Notes, s.ID, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	var id int64
	return r.db.GetContext(ctx, &id, `SELECT id FROM user_schedule WHERE id=? AND user_id=?`, s.ID, userID)
}
func (r *ScheduleRepository) Delete(ctx context.Context, userID int, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM user_schedule WHERE id=? AND user_id=?`, id, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
