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

type applyProjectStub struct{ repository.ProjectRepo }

func (applyProjectStub) GetByID(context.Context, int) (*models.Project, error) {
	return &models.Project{ID: 10, CreatorID: 1, Status: models.ProjectStatusApproved}, nil
}

type applyDuplicateStub struct{ repository.ApplicationRepo }

func (applyDuplicateStub) CheckDuplicate(context.Context, int, int) (bool, error) { return false, nil }

func TestApplyRechecksProjectAndDuplicateAndCommitsNotification(t *testing.T) {
	for _, scenario := range []string{"project closed", "concurrent duplicate", "notification failed", "success"} {
		t.Run(scenario, func(t *testing.T) {
			raw, dbMock, err := sqlmock.New()
			require.NoError(t, err)
			defer raw.Close()
			repo := repository.New(sqlx.NewDb(raw, "sqlmock"))
			repo.Project = applyProjectStub{}
			repo.Application = applyDuplicateStub{}
			repo.User = &oliveSendUserRepo{}
			repo.WxSubscribeDelivery = &oliveSendDeliveryRepo{}
			dbMock.ExpectBegin()
			status := models.ProjectStatusApproved
			if scenario == "project closed" {
				status = models.ProjectStatusPending
			}
			dbMock.ExpectQuery("SELECT status FROM project").WithArgs(10).WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow(status))
			if scenario != "project closed" {
				dbMock.ExpectQuery("SELECT EXISTS").WithArgs(10, 7).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(scenario == "concurrent duplicate"))
				if scenario != "concurrent duplicate" {
					dbMock.ExpectExec("INSERT INTO project_application").WillReturnResult(sqlmock.NewResult(5, 1))
					if scenario == "notification failed" {
						dbMock.ExpectExec("INSERT INTO wx_subscribe_delivery").WillReturnError(errors.New("outbox down"))
					} else {
						dbMock.ExpectExec("INSERT INTO wx_subscribe_delivery").WillReturnResult(sqlmock.NewResult(9, 1))
					}
				}
			}
			if scenario == "success" {
				dbMock.ExpectCommit()
			} else {
				dbMock.ExpectRollback()
			}
			application, err := NewProjectService(repo, nil, NewMessageService(repo, nil)).ApplyToProject(context.Background(), ApplyToProjectInput{ProjectID: 10, UserID: 7})
			if scenario == "success" {
				require.NoError(t, err)
				require.Equal(t, 5, application.ID)
			} else {
				require.Error(t, err)
				require.Nil(t, application)
			}
			require.NoError(t, dbMock.ExpectationsWereMet())
		})
	}
}
