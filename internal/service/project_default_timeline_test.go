package service

import (
	"context"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type defaultTimelineRepo struct {
	*MockProjectRepo
	projectMetadataRepo
	t *testing.T
}

func (r *defaultTimelineRepo) UpdateWithMetadata(ctx context.Context, p *models.Project, tags *[]string, role *string, school *int, milestones *[]models.ProjectMilestone, members *[]models.ProjectMember, events *[]int, user int, images *[]string, reset bool) ([]string, error) {
	require.Nil(r.t, milestones)
	require.False(r.t, reset)
	require.NotNil(r.t, p.DefaultTimelineHiddenUpdate)
	p.DefaultTimelineHidden = *p.DefaultTimelineHiddenUpdate
	return nil, nil
}

func TestHideDefaultTimelinePreservesCreationDateAndStatus(t *testing.T) {
	created := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	p := &models.Project{ID: 42, CreatorID: 7, CreatedAt: created, Status: 1}
	r := &defaultTimelineRepo{MockProjectRepo: new(MockProjectRepo), t: t}
	r.On("GetByID", mock.Anything, 42).Return(p, nil).Twice()
	r.On("GetMemberRole", mock.Anything, 42, 7).Return("", nil).Once()
	svc := NewProjectService(&repository.Repository{Project: r}, nil, nil)
	hidden := true
	result, err := svc.UpdateProject(context.Background(), 42, 7, UpdateProjectInput{DefaultTimelineHidden: &hidden})
	require.NoError(t, err)
	require.True(t, result.DefaultTimelineHidden)
	require.Equal(t, created, result.CreatedAt)
	require.Equal(t, 1, result.Status)
	require.True(t, *result.ToDetailVO().DefaultTimelineHidden)
	r.AssertExpectations(t)
}

func TestMemberCannotHideDefaultTimeline(t *testing.T) {
	r := new(MockProjectRepo)
	r.On("GetByID", mock.Anything, 42).Return(&models.Project{ID: 42, CreatorID: 9}, nil).Once()
	r.On("GetMemberRole", mock.Anything, 42, 7).Return(models.ProjectRoleTeamLeader, nil).Once()
	svc := NewProjectService(&repository.Repository{Project: r}, nil, nil)
	hidden := true
	_, err := svc.UpdateProject(context.Background(), 42, 7, UpdateProjectInput{DefaultTimelineHidden: &hidden})
	var se *ServiceError
	require.ErrorAs(t, err, &se)
	require.Equal(t, ErrCodeForbidden, se.Code)
	r.AssertExpectations(t)
}
