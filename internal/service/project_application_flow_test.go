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

type projectApplicationRepoStub struct {
	repository.ApplicationRepo
	app *models.ProjectApplication
}

func (s projectApplicationRepoStub) GetByID(context.Context, int) (*models.ProjectApplication, error) {
	return s.app, nil
}

func TestApplicationRolePermissionMatrix(t *testing.T) {
	leader := models.ProjectRoleTeamLeader
	tech := "TECH_LEADER"
	member := models.ProjectRoleTeamMember
	reviewerID := 7

	require.True(t, canReviewApplicationByRole(1, &member, &models.ProjectApplication{Status: models.ApplicationStatusPending}))
	require.True(t, canReviewApplicationByRole(reviewerID, &member, &models.ProjectApplication{Status: models.ApplicationStatusDiscussing, ReviewerID: &reviewerID, ReviewerRole: &leader}))
	require.True(t, canReviewApplicationByRole(2, &leader, &models.ProjectApplication{Status: models.ApplicationStatusDiscussing, ReviewerRole: &tech}))
	require.False(t, canReviewApplicationByRole(2, &member, &models.ProjectApplication{Status: models.ApplicationStatusDiscussing, ReviewerRole: &tech}))

	require.True(t, canAssignProjectRole(&leader, "TECH_LEADER"))
	require.True(t, canAssignProjectRole(&tech, models.ProjectRoleTeamMember))
	require.False(t, canAssignProjectRole(&member, "TECH_LEADER"))
}

func TestReviewApplicationRejectsUnsupportedTransitions(t *testing.T) {
	for _, tc := range []struct {
		name   string
		app    *models.ProjectApplication
		status api.ApplicationStatus
	}{
		{name: "joined is terminal", app: &models.ProjectApplication{Status: models.ApplicationStatusJoined}, status: api.ApplicationStatus(models.ApplicationStatusRejected)},
		{name: "discussing cannot discuss again", app: &models.ProjectApplication{Status: models.ApplicationStatusDiscussing}, status: api.ApplicationStatus(models.ApplicationStatusDiscussing)},
		{name: "joined cannot be requested through review", app: &models.ProjectApplication{Status: models.ApplicationStatusPending}, status: api.ApplicationStatus(models.ApplicationStatusJoined)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewProjectService(&repository.Repository{Application: projectApplicationRepoStub{app: tc.app}}, nil, nil)

			err := svc.ReviewApplication(context.Background(), 1, 2, tc.status)

			var serviceErr *ServiceError
			require.ErrorAs(t, err, &serviceErr)
			require.Equal(t, ErrCodeBadRequest, serviceErr.Code)
		})
	}
}

func TestAssignApplicationRoleRequiresDiscussingApplication(t *testing.T) {
	svc := NewProjectService(&repository.Repository{
		Application: projectApplicationRepoStub{app: &models.ProjectApplication{Status: models.ApplicationStatusPending}},
	}, nil, nil)

	err := svc.AssignApplicationRole(context.Background(), AssignApplicationRoleInput{ApplicationID: 1, UserID: 2, Role: models.ProjectRoleTeamMember})

	var serviceErr *ServiceError
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, ErrCodeBadRequest, serviceErr.Code)
}

func TestAssignApplicationRoleRejectsLowerRoleAssigningHigherRole(t *testing.T) {
	projectRepo := new(MockProjectRepo)
	const projectID = 42
	const reviewerID = 7
	projectRepo.On("GetByID", mock.Anything, projectID).Return(&models.Project{ID: projectID, CreatorID: 99}, nil).Once()
	projectRepo.On("ListMembers", mock.Anything, projectID).Return([]models.ProjectMember{{UserID: reviewerID, Role: models.ProjectRoleTeamMember}}, nil).Once()

	svc := NewProjectService(&repository.Repository{
		Application: projectApplicationRepoStub{app: &models.ProjectApplication{ProjectID: projectID, Status: models.ApplicationStatusDiscussing}},
		Project:     projectRepo,
	}, nil, nil)

	err := svc.AssignApplicationRole(context.Background(), AssignApplicationRoleInput{ApplicationID: 1, UserID: reviewerID, Role: "TECH_LEADER"})

	var serviceErr *ServiceError
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, ErrCodeForbidden, serviceErr.Code)
	projectRepo.AssertExpectations(t)
}
