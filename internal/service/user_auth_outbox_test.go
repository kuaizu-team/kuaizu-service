package service

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestUserAuthReviewNotificationFailureAndConflictRollBack(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(map[bool]string{false: "outbox failure", true: "concurrent review"}[conflict], func(t *testing.T) {
			raw, dbMock, err := sqlmock.New()
			require.NoError(t, err)
			defer raw.Close()
			repo := repository.New(sqlx.NewDb(raw, "sqlmock"))
			repo.User = &oliveSendUserRepo{}
			dbMock.ExpectBegin()
			affected := int64(1)
			if conflict {
				affected = 0
			}
			dbMock.ExpectExec("UPDATE .*user.* SET auth_status").WithArgs(1, 7, nil).WillReturnResult(sqlmock.NewResult(0, affected))
			if !conflict {
				dbMock.ExpectExec("INSERT INTO wx_subscribe_delivery").WillReturnError(errors.New("outbox down"))
			}
			dbMock.ExpectRollback()
			err = NewUserService(repo, NewMessageService(repo, nil)).ReviewUserAuth(context.Background(), 7, 1)
			require.Error(t, err)
			require.NoError(t, dbMock.ExpectationsWereMet())
		})
	}
}
