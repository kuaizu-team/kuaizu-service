package service

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestApplicationNotificationFailureRollsBackReview(t *testing.T) {
	raw, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	defer raw.Close()
	repo := repository.New(sqlx.NewDb(raw, "sqlmock"))
	repo.Application = projectApplicationRepoStub{app: &models.ProjectApplication{ID: 5, ProjectID: 42, UserID: 8, Status: 0}}
	projects := new(MockProjectRepo)
	projects.On("GetByID", mock.Anything, 42).Return(&models.Project{ID: 42, CreatorID: 2, Name: "Project"}, nil).Once()
	projects.On("ListMembers", mock.Anything, 42).Return([]models.ProjectMember{{UserID: 2, Role: models.ProjectRoleTeamLeader}}, nil).Once()
	repo.Project = projects
	dbMock.ExpectBegin()
	dbMock.ExpectExec("WHERE id = \\? AND status = \\? AND reviewer_id").WithArgs(1, 2, models.ProjectRoleTeamLeader, 1, 1, 1, 2, 5, 0, nil, nil).WillReturnResult(sqlmock.NewResult(0, 1))
	dbMock.ExpectExec("INSERT INTO wx_subscribe_delivery").WillReturnError(errors.New("outbox unavailable"))
	dbMock.ExpectRollback()
	err = NewProjectService(repo, nil, NewMessageService(repo, nil)).ReviewApplication(context.Background(), 5, 2, api.ApplicationStatus(1))
	require.Error(t, err)
	require.NoError(t, dbMock.ExpectationsWereMet())
}

func TestApplicationCycleFailureRollsBackBusinessAndNotification(t *testing.T) {
	raw, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	defer raw.Close()
	repo := repository.New(sqlx.NewDb(raw, "sqlmock"))
	repo.AutoUrge.EnableTracking()
	repo.Application = projectApplicationRepoStub{app: &models.ProjectApplication{ID: 5, ProjectID: 42, UserID: 8, Status: 0}}
	projects := new(MockProjectRepo)
	projects.On("GetByID", mock.Anything, 42).Return(&models.Project{ID: 42, CreatorID: 2, Name: "Project"}, nil).Once()
	projects.On("ListMembers", mock.Anything, 42).Return([]models.ProjectMember{{UserID: 2, Role: models.ProjectRoleTeamLeader}}, nil).Once()
	repo.Project = projects
	dbMock.ExpectBegin()
	dbMock.ExpectExec("WHERE id = \\? AND status = \\? AND reviewer_id").WithArgs(1, 2, models.ProjectRoleTeamLeader, 1, 1, 1, 2, 5, 0, nil, nil).WillReturnResult(sqlmock.NewResult(0, 1))
	dbMock.ExpectExec("INSERT INTO wx_subscribe_delivery").WillReturnResult(sqlmock.NewResult(12, 1))
	dbMock.ExpectQuery("SELECT creator_id AS user_id FROM project").WithArgs(42, 42, 42).WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(2))
	dbMock.ExpectExec("UPDATE auto_urge_sms_state a SET closed_at").WithArgs(2).WillReturnError(errors.New("cycle state unavailable"))
	dbMock.ExpectRollback()
	err = NewProjectService(repo, nil, NewMessageService(repo, nil)).ReviewApplication(context.Background(), 5, 2, api.ApplicationStatus(1))
	require.Error(t, err)
	require.NoError(t, dbMock.ExpectationsWereMet())
}
