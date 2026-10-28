package repository

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"testing"
)

func TestEventTagsStoredWithLibraryAtomically(t *testing.T) {
	for _, update := range []bool{false, true} {
		for _, fail := range []bool{false, true} {
			raw, m, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			db := sqlx.NewDb(raw, "sqlmock")
			tags := `["算法设计","科技创新","团队协作"]`
			event := &models.Event{ID: 42, Name: "赛事", TagsJSON: &tags}
			m.ExpectBegin()
			query := "(?s)INSERT INTO event.*tags.*VALUES"
			if update {
				query = "(?s)UPDATE event.*tags = "
			}
			m.ExpectExec(query).WillReturnResult(sqlmock.NewResult(42, 1))
			m.ExpectExec("(?s)INSERT INTO project_tag.*ON DUPLICATE KEY UPDATE").WithArgs("算法设计").WillReturnResult(sqlmock.NewResult(1, 1))
			next := m.ExpectExec("(?s)INSERT INTO project_tag.*ON DUPLICATE KEY UPDATE").WithArgs("科技创新")
			if fail {
				next.WillReturnError(errors.New("tag library unavailable"))
				m.ExpectRollback()
			} else {
				next.WillReturnResult(sqlmock.NewResult(2, 1))
				m.ExpectExec("(?s)INSERT INTO project_tag.*ON DUPLICATE KEY UPDATE").WithArgs("团队协作").WillReturnResult(sqlmock.NewResult(3, 1))
				m.ExpectCommit()
			}
			repo := NewEventRepository(db)
			if update {
				err = repo.Update(context.Background(), event)
			} else {
				err = repo.Create(context.Background(), event)
			}
			if (err != nil) != fail {
				t.Fatalf("update=%v fail=%v: %v", update, fail, err)
			}
			if err := m.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
			db.Close()
		}
	}
}

func TestEventTagsReadFromDatabase(t *testing.T) {
	raw, m, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	m.ExpectQuery("(?s)SELECT.*event.tags.*FROM event").WillReturnRows(sqlmock.NewRows([]string{"id", "name", "tags"}).AddRow(42, "赛事", `["算法设计","科技创新","团队协作"]`))
	event, err := NewEventRepository(sqlx.NewDb(raw, "sqlmock")).GetByID(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if event == nil || len(event.TagNames()) != 3 {
		t.Fatalf("tags not loaded: %#v", event)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
