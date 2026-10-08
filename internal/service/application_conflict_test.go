package service

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestConcurrentApplicationChangeRollsBackWithoutNotification(t *testing.T) {
	raw, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	defer raw.Close()
	repo := repository.New(sqlx.NewDb(raw, "sqlmock"))
	repo.Application = projectApplicationRepoStub{app: &models.ProjectApplication{ID: 5, ProjectID: 42, UserID: 8, Status: 1}}
	projects := new(MockProjectRepo)
	projects.On("GetByID", mock.Anything, 42).Return(&models.Project{ID: 42, CreatorID: 2}, nil).Once()
	projects.On("ListMembers", mock.Anything, 42).Return([]models.ProjectMember{{UserID: 2, Role: models.ProjectRoleTeamLeader}}, nil).Once()
	repo.Project = projects
	dbMock.ExpectBegin()
	dbMock.ExpectExec("WHERE id = \\? AND status = \\? AND reviewer_id").
		WithArgs(2, 2, models.ProjectRoleTeamLeader, 2, 1, 2, 2, 5, 1, nil, nil).
		WillReturnResult(sqlmock.NewResult(0, 0))
	dbMock.ExpectRollback()
	err = NewProjectService(repo, nil, nil).ReviewApplication(context.Background(), 5, 2, api.ApplicationStatus(2))
	var serviceErr *ServiceError
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, ErrCodeBadRequest, serviceErr.Code)
	require.NoError(t, dbMock.ExpectationsWereMet())
}
