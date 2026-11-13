package repository

import (
	"context"
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
			m.ExpectExec("INSERT INTO user_schedule .*import_key").WithArgs(7, "数学", "2026-11-16", true, nil, "", "", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
			second := m.ExpectExec("INSERT INTO user_schedule .*import_key").WithArgs(7, "英语", "2026-11-17", true, nil, "", "", sqlmock.AnyArg())
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
