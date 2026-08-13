package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestUpdateTalentProfileStatusIfCurrent(t *testing.T) {
	for _, tc := range []struct {
		name         string
		rowsAffected int64
		wantUpdated  bool
	}{
		{name: "status matches", rowsAffected: 1, wantUpdated: true},
		{name: "status changed", rowsAffected: 0, wantUpdated: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer raw.Close()

			repo := NewTalentProfileRepository(sqlx.NewDb(raw, "sqlmock"))
			mock.ExpectExec(regexp.QuoteMeta(`
				UPDATE talent_profile
				SET status = ?, reject_reason = ?, updated_at = CURRENT_TIMESTAMP
				WHERE id = ? AND status = ?
			`)).WithArgs(0, nil, 42, 2).
				WillReturnResult(sqlmock.NewResult(0, tc.rowsAffected))

			updated, err := repo.UpdateStatusIfCurrent(context.Background(), 42, 2, 0, nil)
			if err != nil {
				t.Fatal(err)
			}
			if updated != tc.wantUpdated {
				t.Fatalf("updated = %v, want %v", updated, tc.wantUpdated)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
