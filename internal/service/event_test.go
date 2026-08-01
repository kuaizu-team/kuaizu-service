package service

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type eventRepoStub struct {
	event *models.Event
}

func (s eventRepoStub) List(context.Context, repository.EventListParams) ([]models.Event, int64, error) {
	return nil, 0, nil
}
func (s eventRepoStub) ListTimeline(context.Context, int) ([]models.Event, error) { return nil, nil }
func (s eventRepoStub) GetByID(context.Context, int) (*models.Event, error)       { return s.event, nil }
func (s eventRepoStub) GetByIDWithProjectSchoolIDs(context.Context, int, []int) (*models.Event, error) {
	return s.event, nil
}
func (s eventRepoStub) Create(context.Context, *models.Event) error { return nil }
func (s eventRepoStub) Update(context.Context, *models.Event) error { return nil }
func (s eventRepoStub) Delete(context.Context, int) error           { return nil }
func (s eventRepoStub) Merge(context.Context, int, int) error       { return nil }
func (s eventRepoStub) ListByProjectIDs(context.Context, []int) (map[int][]models.Event, error) {
	return nil, nil
}
func (s eventRepoStub) ListProjectIDs(context.Context, int) ([]int, error) { return nil, nil }
func (s eventRepoStub) ReplaceProjectEventsTx(context.Context, *sqlx.Tx, int, []int) error {
	return nil
}

type eventProjectRepoStub struct {
	*MockProjectRepo
	pages []int
}

func (s *eventProjectRepoStub) List(_ context.Context, params repository.ListParams) ([]models.Project, int64, error) {
	s.pages = append(s.pages, params.Page)
	if params.Page == 1 {
		return make([]models.Project, 1000), 1001, nil
	}
	return make([]models.Project, 1), 1001, nil
}

func TestGetEventLoadsAllApprovedProjectsAcrossPages(t *testing.T) {
	projectRepo := &eventProjectRepoStub{MockProjectRepo: &MockProjectRepo{}}
	repo := &repository.Repository{
		Event:   eventRepoStub{event: &models.Event{ID: 8}},
		Project: projectRepo,
	}
	svc := NewEventService(repo)

	event, projects, err := svc.GetEvent(context.Background(), 8)

	require.NoError(t, err)
	require.NotNil(t, event)
	assert.Len(t, projects, 1001)
	assert.Equal(t, []int{1, 2}, projectRepo.pages)
}
