package repository

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"regexp"
	"testing"
)

func TestProjectDetailApplicationCount(t *testing.T) {
	for _, count := range []int{0, 4} {
		raw, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		repo := NewProjectRepository(sqlx.NewDb(raw, "mysql"))
		mock.ExpectQuery(`(?s)SELECT.*` + regexp.QuoteMeta("(SELECT COUNT(*) FROM project_application pa WHERE pa.project_id=p.id) AS application_count") + `.*WHERE p.id = \?`).WithArgs(42).WillReturnRows(sqlmock.NewRows([]string{"id", "application_count"}).AddRow(42, count))
		mock.ExpectQuery(`SELECT r.project_id,t.id,t.name FROM project_tag_relation`).WithArgs(42).WillReturnRows(sqlmock.NewRows([]string{"project_id", "id", "name"}))
		project, err := repo.GetByID(context.Background(), 42)
		if err != nil {
			t.Fatal(err)
		}
		got := project.ToDetailVO().ApplicationCount
		if got == nil || *got != count {
			t.Fatalf("count=%v want %d", got, count)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		raw.Close()
	}
}
