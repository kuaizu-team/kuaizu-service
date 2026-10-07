package service

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestOliveLifecycleTimestampsAndNotificationTransaction(t *testing.T) {
	for _, action := range []string{"ACCEPT", "REJECT"} {
		for _, fail := range []bool{false, true} {
			t.Run(action+map[bool]string{false: " committed", true: " rolled back"}[fail], func(t *testing.T) {
				raw, dbMock, err := sqlmock.New()
				require.NoError(t, err)
				defer raw.Close()
				repo := repository.New(sqlx.NewDb(raw, "sqlmock"))
				repo.OliveBranch = oliveConflictRepo{}
				repo.User = &oliveSendUserRepo{}
				repo.WxSubscribeDelivery = &oliveSendDeliveryRepo{}
				dbMock.ExpectBegin()
				dbMock.ExpectQuery("SELECT status FROM olive_branch_record").WithArgs(5).WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow(0))
				status := models.OliveBranchStatusDiscussing
				if action == "REJECT" {
					status = models.OliveBranchStatusRejected
				}
				dbMock.ExpectExec("(?s)UPDATE olive_branch_record SET status.*discussing_at=CASE.*rejected_at=CASE").
					WithArgs(status, status, models.OliveBranchStatusDiscussing, status, models.OliveBranchStatusRejected, 5).
					WillReturnResult(sqlmock.NewResult(0, 1))
				if fail {
					dbMock.ExpectExec("INSERT INTO wx_subscribe_delivery").WillReturnError(errors.New("outbox down"))
					dbMock.ExpectRollback()
				} else {
					dbMock.ExpectExec("INSERT INTO wx_subscribe_delivery").WillReturnResult(sqlmock.NewResult(9, 1))
					dbMock.ExpectCommit()
				}
				ob, err := NewOliveBranchService(repo, NewMessageService(repo, nil)).HandleOliveBranch(context.Background(), 7, 5, action, "")
				if fail {
					require.Error(t, err)
					require.Nil(t, ob)
				} else {
					require.NoError(t, err)
					require.Equal(t, status, ob.Status)
				}
				require.NoError(t, dbMock.ExpectationsWereMet())
			})
		}
	}
}
