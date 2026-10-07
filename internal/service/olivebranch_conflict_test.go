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

type oliveConflictRepo struct{ repository.OliveBranchRepo }

func (oliveConflictRepo) GetByID(context.Context, int) (*models.OliveBranch, error) {
	return &models.OliveBranch{ID: 5, SenderID: 1, ReceiverID: 7, Status: 0}, nil
}
func TestOliveConcurrentStatusChangeCannotBeOverwritten(t *testing.T) {
	raw, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer raw.Close()
	repo := repository.New(sqlx.NewDb(raw, "sqlmock"))
	repo.OliveBranch = oliveConflictRepo{}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT status FROM olive_branch_record WHERE id=\\? FOR UPDATE").WithArgs(5).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow(1))
	mock.ExpectRollback()
	_, err = NewOliveBranchService(repo, nil).HandleOliveBranch(context.Background(), 7, 5, "REJECT", "")
	var serviceErr *ServiceError
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, ErrCodeBadRequest, serviceErr.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}
