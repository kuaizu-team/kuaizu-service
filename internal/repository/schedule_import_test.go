package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

func TestScheduleImportAtomicAndDuplicate(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			db, m, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			repo := NewScheduleRepository(sqlx.NewDb(db, "sqlmock"))
			items := []models.Schedule{{Title: "数学", Date: "2026-11-16", AllDay: true}, {Title: "英语", Date: "2026-11-17", AllDay: true}}
			m.ExpectBegin()
			m.ExpectExec("INSERT INTO user_schedule .*import_key").WithArgs(7, "数学", "2026-11-16", true, nil, "", "", nil, nil, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
			second := m.ExpectExec("INSERT INTO user_schedule .*import_key").WithArgs(7, "英语", "2026-11-17", true, nil, "", "", nil, nil, sqlmock.AnyArg())
			if fail {
				second.WillReturnError(fmt.Errorf("write failed"))
				m.ExpectRollback()
			} else {
				second.WillReturnError(&mysql.MySQLError{Number: 1062})
				m.ExpectCommit()
			}
			created, skipped, err := repo.Import(context.Background(), 7, items)
			if fail && (err == nil || created != 0 || skipped != 0) {
				t.Fatalf("partial result: %d %d %v", created, skipped, err)
			}
			if !fail && (err != nil || created != 1 || skipped != 1) {
				t.Fatalf("unexpected result: %d %d %v", created, skipped, err)
			}
			if err := m.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDurationImportPreservesLegacyFingerprint(t *testing.T) {
	db, m, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewScheduleRepository(sqlx.NewDb(db, "sqlmock"))
	date, clock, end := "2026-11-16", "08:00", "09:30"
	notes := "结束：2026-11-16 09:30"
	legacy := `{"id":0,"title":"数学","date":"2026-11-16","allDay":false,"time":"08:00","location":"A101","notes":"结束：2026-11-16 09:30"}`
	hash := sha256.Sum256([]byte(legacy))
	key := hex.EncodeToString(hash[:])
	item := models.Schedule{Title: "数学", Date: date, Time: &clock, EndDate: &date, EndTime: &end, Location: "A101", Notes: notes, Completed: true}
	m.ExpectBegin()
	m.ExpectExec("INSERT INTO user_schedule .*import_key").WithArgs(7, item.Title, date, false, clock, item.Location, notes, date, end, key).WillReturnError(&mysql.MySQLError{Number: 1062})
	m.ExpectExec("UPDATE user_schedule SET end_date=.*end_time IS NULL AND title=.*notes=").WithArgs(date, end, 7, key, item.Title, date, false, clock, item.Location, notes).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectCommit()
	created, skipped, err := repo.Import(context.Background(), 7, []models.Schedule{item})
	if err != nil || created != 0 || skipped != 1 {
		t.Fatalf("%d %d %v", created, skipped, err)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
