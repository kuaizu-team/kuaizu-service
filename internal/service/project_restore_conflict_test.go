package service

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRestoreOnlyChangesStillDeletingProject(t *testing.T) {
	for _, affected := range []int64{0, 1} {
		t.Run(map[int64]string{0: "already changed or purged", 1: "still deleting"}[affected], func(t *testing.T) {
			raw, dbMock, err := sqlmock.New()
			require.NoError(t, err)
			defer raw.Close()
			repo := repository.New(sqlx.NewDb(raw, "sqlmock"))
			dbMock.ExpectBegin()
			dbMock.ExpectExec("(?s)UPDATE project SET.*WHERE id=\\? AND status=\\?").WillReturnResult(sqlmock.NewResult(0, affected))
			if affected == 0 {
				dbMock.ExpectRollback()
			} else {
				dbMock.ExpectCommit()
			}
			err = NewProjectService(repo, nil, nil).restoreProject(context.Background(), 7, &models.Project{ID: 7, Status: models.ProjectStatusDeleting})
			if affected == 0 {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, dbMock.ExpectationsWereMet())
		})
	}
}
