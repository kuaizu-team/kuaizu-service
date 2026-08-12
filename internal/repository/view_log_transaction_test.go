package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/stretchr/testify/require"
)

func TestProjectRecordViewTransaction(t *testing.T) {
	testRecordViewTransaction(t, "project_view_log", "project", func(db *sqlx.DB) error {
		return NewProjectViewLogRepository(db).RecordView(context.Background(),
			&models.ProjectViewLog{ProjectID: 42, Source: models.ViewSourceList})
	})
}

func TestTalentRecordViewTransaction(t *testing.T) {
	testRecordViewTransaction(t, "talent_view_log", "talent_profile", func(db *sqlx.DB) error {
		return NewTalentViewLogRepository(db).RecordView(context.Background(),
			&models.TalentViewLog{TalentID: 42, Source: models.ViewSourceList})
	})
}

func testRecordViewTransaction(t *testing.T, logTable, targetTable string, record func(*sqlx.DB) error) {
	t.Helper()
	for _, tc := range []struct {
		name       string
		insertErr  error
		updateErr  error
		updateRows int64
		commitErr  error
		wantErr    bool
	}{
		{name: "commit", updateRows: 1},
		{name: "insert rollback", insertErr: errors.New("insert failed"), wantErr: true},
		{name: "counter rollback", updateErr: errors.New("update failed"), wantErr: true},
		{name: "missing target rollback", updateRows: 0, wantErr: true},
		{name: "commit failure", updateRows: 1, commitErr: errors.New("commit failed"), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer raw.Close()
			db := sqlx.NewDb(raw, "sqlmock")
			mock.ExpectBegin()
			insert := mock.ExpectExec("INSERT INTO " + logTable)
			if tc.insertErr != nil {
				insert.WillReturnError(tc.insertErr)
				mock.ExpectRollback()
			} else {
				insert.WillReturnResult(sqlmock.NewResult(1, 1))
				update := mock.ExpectExec("UPDATE " + targetTable + " SET view_count")
				if tc.updateErr != nil {
					update.WillReturnError(tc.updateErr)
					mock.ExpectRollback()
				} else {
					update.WillReturnResult(sqlmock.NewResult(0, tc.updateRows))
					if tc.updateRows == 0 {
						mock.ExpectRollback()
					} else if tc.commitErr != nil {
						mock.ExpectCommit().WillReturnError(tc.commitErr)
					} else {
						mock.ExpectCommit()
					}
				}
			}

			err = record(db)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
