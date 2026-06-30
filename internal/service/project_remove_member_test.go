package service

import (
	"context"
	"testing"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func removeScorePtr(v int) *int { return &v }

func TestRemoveMemberRejectsInvalidInputBeforeRepository(t *testing.T) {
	svc := NewProjectService(&repository.Repository{}, nil, nil)

	for _, tc := range []struct {
		name      string
		projectID int
		scorerID  int
		memberID  int
		score     *int
		message   string
	}{
		{name: "invalid project", projectID: 0, scorerID: 1, memberID: 2, score: removeScorePtr(80), message: "invalid project or member id"},
		{name: "invalid member", projectID: 1, scorerID: 2, memberID: 0, score: removeScorePtr(80), message: "invalid project or member id"},
		{name: "low score", projectID: 1, scorerID: 2, memberID: 3, score: removeScorePtr(-1), message: "score must be between 0 and 100"},
		{name: "high score", projectID: 1, scorerID: 2, memberID: 3, score: removeScorePtr(101), message: "score must be between 0 and 100"},
		{name: "self remove", projectID: 1, scorerID: 2, memberID: 2, score: removeScorePtr(80), message: "不能移除自己"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.RemoveMember(context.Background(), tc.projectID, tc.scorerID, tc.memberID, tc.score)

			var serviceErr *ServiceError
			require.ErrorAs(t, err, &serviceErr)
			require.Equal(t, ErrCodeBadRequest, serviceErr.Code)
			require.Equal(t, tc.message, serviceErr.Message)
		})
	}
}

func TestRemoveMemberRejectsNonHighestRole(t *testing.T) {
	projectRepo := new(MockProjectRepo)
	const projectID = 42
	const scorerID = 7
	const memberID = 8

	projectRepo.On("GetByID", mock.Anything, projectID).Return(&models.Project{
		ID:        projectID,
		CreatorID: 99,
	}, nil).Once()
	projectRepo.On("ListMembers", mock.Anything, projectID).Return([]models.ProjectMember{
		{UserID: scorerID, Role: models.ProjectRoleTeamMember},
		{UserID: memberID, Role: models.ProjectRoleTeamMember},
		{UserID: 9, Role: models.ProjectRoleTeamLeader},
	}, nil).Once()

	svc := NewProjectService(&repository.Repository{Project: projectRepo}, nil, nil)
	_, err := svc.RemoveMember(context.Background(), projectID, scorerID, memberID, removeScorePtr(80))

	var serviceErr *ServiceError
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, ErrCodeForbidden, serviceErr.Code)
	require.Equal(t, "当前角色不能移除项目成员", serviceErr.Message)
	projectRepo.AssertExpectations(t)
}
