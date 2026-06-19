package service

import (
	"context"
	"testing"

	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type projectUpdateUserRepoStub struct {
	repository.UserRepo
}

func (projectUpdateUserRepoStub) GetByID(_ context.Context, id int) (*models.User, error) {
	return &models.User{ID: id}, nil
}

func TestUpdateProjectRejectsRemovingCurrentUserFromMembers(t *testing.T) {
	projectRepo := new(MockProjectRepo)
	const projectID = 42
	const userID = 7

	projectRepo.On("GetByID", mock.Anything, projectID).Return(&models.Project{
		ID:        projectID,
		CreatorID: 9,
	}, nil).Once()
	projectRepo.On("GetMemberRole", mock.Anything, projectID, userID).Return(models.ProjectRoleTeamLeader, nil).Once()
	projectRepo.On("RoleExists", mock.Anything, models.ProjectRoleTeamMember).Return(true, nil).Once()

	svc := NewProjectService(&repository.Repository{
		Project: projectRepo,
		User:    projectUpdateUserRepoStub{},
	}, nil, nil)
	members := []api.ProjectMemberDTO{
		{UserId: 8, Role: models.ProjectRoleTeamMember},
	}

	_, err := svc.UpdateProject(context.Background(), projectID, userID, UpdateProjectInput{Members: &members})

	var serviceErr *ServiceError
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, ErrCodeBadRequest, serviceErr.Code)
	require.Equal(t, "不能删除自己", serviceErr.Message)
	projectRepo.AssertNotCalled(t, "ReplaceMembers", mock.Anything, projectID, mock.Anything)
	projectRepo.AssertExpectations(t)
}
