package service

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/stretchr/testify/require"
)

type projectDetailRepoSpy struct {
	repository.ProjectRepo
	incrementCalls atomic.Int32
}

func (r *projectDetailRepoSpy) GetByID(_ context.Context, id int) (*models.Project, error) {
	return &models.Project{ID: id, CreatorID: 2}, nil
}

func (r *projectDetailRepoSpy) ListMilestones(_ context.Context, _ int) ([]models.ProjectMilestone, error) {
	return nil, nil
}

func (r *projectDetailRepoSpy) ListMembers(_ context.Context, projectID int) ([]models.ProjectMember, error) {
	return []models.ProjectMember{{
		ProjectID: projectID,
		UserID:    2,
		Role:      models.ProjectRoleTeamLeader,
	}}, nil
}

func (r *projectDetailRepoSpy) IncrementViewCount(_ context.Context, _ int) error {
	r.incrementCalls.Add(1)
	return nil
}

type projectViewLogSpy struct {
	repository.ProjectViewLogRepo
	insertCalls atomic.Int32
	notifyCalls atomic.Int32
}

func (r *projectViewLogSpy) InsertViewLog(_ context.Context, _ *models.ProjectViewLog) error {
	r.insertCalls.Add(1)
	return nil
}

func (r *projectViewLogSpy) NotifyProgress(_ context.Context, _, _, _ int) (repository.InteractionNotifyProgress, error) {
	r.notifyCalls.Add(1)
	return repository.InteractionNotifyProgress{}, nil
}

func TestGetProjectDetailCreatorRefreshKeepsPermissionsAndSkipsViewSideEffects(t *testing.T) {
	projectRepo := &projectDetailRepoSpy{}
	viewLogRepo := &projectViewLogSpy{}
	svc := NewProjectService(&repository.Repository{
		Project:        projectRepo,
		ProjectViewLog: viewLogRepo,
	}, nil, nil)

	project, err := svc.GetProjectDetail(context.Background(), 42, 2, 0, false)

	require.NoError(t, err)
	require.NotNil(t, project.CurrentUserRole)
	require.Equal(t, models.ProjectRoleTeamLeader, *project.CurrentUserRole)
	require.NotNil(t, project.CanCompleteRecruitment)
	require.True(t, *project.CanCompleteRecruitment)
	require.Zero(t, projectRepo.incrementCalls.Load())
	require.Zero(t, viewLogRepo.insertCalls.Load())
	require.Zero(t, viewLogRepo.notifyCalls.Load())
}

func TestShouldRecordProjectView(t *testing.T) {
	tests := []struct {
		name          string
		recordView    bool
		viewerUserID  int
		creatorUserID int
		want          bool
	}{
		{name: "default recording", recordView: true, viewerUserID: 2, creatorUserID: 2, want: true},
		{name: "creator refresh", recordView: false, viewerUserID: 2, creatorUserID: 2, want: false},
		{name: "anonymous request", recordView: false, viewerUserID: 0, creatorUserID: 2, want: true},
		{name: "non-creator request", recordView: false, viewerUserID: 3, creatorUserID: 2, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, shouldRecordProjectView(tt.recordView, tt.viewerUserID, tt.creatorUserID))
		})
	}
}
