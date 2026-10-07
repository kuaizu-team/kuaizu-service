package repository

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"testing"
)

func TestMissingEventCannotEraseExistingAssociations(t *testing.T) {
	raw, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	db := sqlx.NewDb(raw, "mysql")
	mock.ExpectBegin()
	tx, err := db.BeginTxx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT id FROM project WHERE id=").WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery("SELECT id FROM event WHERE id IN").WithArgs(9, 10).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(9))
	mock.ExpectRollback()
	if err := replaceProjectEventsTx(context.Background(), tx, 7, []int{9, 10}); err == nil {
		t.Fatal("missing event must fail before deletion")
	}
	tx.Rollback()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
