package repository

import (
	"context"
	"database/sql"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"testing"
)

func TestScheduleOwnershipAndIdenticalUpdate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := NewScheduleRepository(sqlx.NewDb(db, "sqlmock"))
	ctx := context.Background()
	mock.ExpectQuery("SELECT .* FROM user_schedule WHERE user_id=\\?").WithArgs(7, "2026-11-01", "2026-11-30").WillReturnRows(sqlmock.NewRows([]string{"id", "title", "date", "all_day", "time", "location", "notes"}))
	if _, err := r.List(ctx, 7, "2026-11-01", "2026-11-30"); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("DELETE FROM user_schedule WHERE id=\\? AND user_id=\\?").WithArgs(int64(9), 7).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := r.Delete(ctx, 7, 9); err != sql.ErrNoRows {
		t.Fatalf("other user's item: %v", err)
	}
	mock.ExpectExec("UPDATE user_schedule .* WHERE id=\\? AND user_id=\\?").WithArgs("test", "2026-11-13", true, nil, "", "", int64(9), 7).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT id FROM user_schedule WHERE id=\\? AND user_id=\\?").WithArgs(int64(9), 7).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(9))
	if err := r.Update(ctx, 7, &models.Schedule{ID: 9, Title: "test", Date: "2026-11-13", AllDay: true}); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
