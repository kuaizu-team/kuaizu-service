package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

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
		// Keep the original fingerprint stable; mutable completion is excluded.
		legacy := struct {
			ID       int64   `json:"id"`
			Title    string  `json:"title"`
			Date     string  `json:"date"`
			AllDay   bool    `json:"allDay"`
			Time     *string `json:"time"`
			Location string  `json:"location"`
			Notes    string  `json:"notes"`
		}{0, item.Title, item.Date, item.AllDay, item.Time, item.Location, item.Notes}
		if item.EndDate != nil && item.EndTime != nil {
			marker := "结束：" + *item.EndDate + " " + *item.EndTime
			if !strings.HasSuffix(legacy.Notes, marker) {
				legacy.Notes = strings.TrimSpace(legacy.Notes + "\n" + marker)
			}
		}
		payload, err := json.Marshal(legacy)
		if err != nil {
			return 0, 0, err
		}
		hash := sha256.Sum256(payload)
		key := hex.EncodeToString(hash[:])
		_, err = tx.ExecContext(ctx, `INSERT INTO user_schedule (user_id,title,schedule_date,all_day,schedule_time,location,notes,end_date,end_time,import_key) VALUES (?,?,?,?,?,?,?,?,?,?)`, userID, item.Title, item.Date, item.AllDay, item.Time, item.Location, item.Notes, item.EndDate, item.EndTime, key)
		var duplicate *mysql.MySQLError
		if errors.As(err, &duplicate) && duplicate.Number == 1062 {
			// Fill a legacy import's end only if original content still matches.
			if item.EndDate != nil && item.EndTime != nil {
				_, err = tx.ExecContext(ctx, `UPDATE user_schedule SET end_date=?,end_time=? WHERE user_id=? AND import_key=? AND end_date IS NULL AND end_time IS NULL AND title=? AND schedule_date=? AND all_day=? AND schedule_time=? AND location=? AND notes=?`, item.EndDate, item.EndTime, userID, key, item.Title, item.Date, item.AllDay, item.Time, item.Location, item.Notes)
				if err != nil {
					return 0, 0, err
				}
			}
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
	err := r.db.SelectContext(ctx, &items, `SELECT id,title,DATE_FORMAT(schedule_date,'%Y-%m-%d') AS date,all_day,TIME_FORMAT(schedule_time,'%H:%i') AS time,location,notes,DATE_FORMAT(end_date,'%Y-%m-%d') AS end_date,TIME_FORMAT(end_time,'%H:%i') AS end_time,completed FROM user_schedule WHERE user_id=? AND schedule_date<=? AND (schedule_date>=? OR end_date>? OR (end_date=? AND end_time>'00:00:00')) ORDER BY schedule_date,all_day DESC,schedule_time,id LIMIT 2001`, userID, to, from, from, from)
	return items, err
}
func (r *ScheduleRepository) Create(ctx context.Context, userID int, s *models.Schedule) error {
	res, err := r.db.ExecContext(ctx, `INSERT INTO user_schedule (user_id,title,schedule_date,all_day,schedule_time,location,notes,end_date,end_time,completed) VALUES (?,?,?,?,?,?,?,?,?,?)`, userID, s.Title, s.Date, s.AllDay, s.Time, s.Location, s.Notes, s.EndDate, s.EndTime, s.Completed)
	if err != nil {
		return err
	}
	s.ID, err = res.LastInsertId()
	return err
}
func (r *ScheduleRepository) Update(ctx context.Context, userID int, s *models.Schedule, completion *bool) error {
	// updated_at also changes for real changes. An identical PUT must still succeed.
	_, err := r.db.ExecContext(ctx, `UPDATE user_schedule SET title=?,schedule_date=?,all_day=?,schedule_time=?,location=?,notes=?,end_date=?,end_time=?,completed=COALESCE(?,completed) WHERE id=? AND user_id=?`, s.Title, s.Date, s.AllDay, s.Time, s.Location, s.Notes, s.EndDate, s.EndTime, completion, s.ID, userID)
	if err != nil {
		return err
	}
	return r.db.GetContext(ctx, &s.Completed, `SELECT completed FROM user_schedule WHERE id=? AND user_id=?`, s.ID, userID)
}
func (r *ScheduleRepository) Complete(ctx context.Context, userID int, id int64, completed bool) error {
	_, err := r.db.ExecContext(ctx, `UPDATE user_schedule SET completed=? WHERE id=? AND user_id=?`, completed, id, userID)
	if err != nil {
		return err
	}
	var found int64
	return r.db.GetContext(ctx, &found, `SELECT id FROM user_schedule WHERE id=? AND user_id=?`, id, userID)
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
