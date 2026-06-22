package service

import (
	"context"
	"log"
	"strings"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

type EventService struct {
	repo *repository.Repository
}

type EventListResult struct {
	List       []models.Event
	Total      int64
	TotalPages int
	Page       int
	Size       int
}

func NewEventService(repo *repository.Repository) *EventService {
	return &EventService{repo: repo}
}

func (s *EventService) ListEvents(ctx context.Context, params repository.EventListParams) (*EventListResult, error) {
	params.Page, params.Size = normalizePageParams(params.Page, params.Size)
	events, total, err := s.repo.Event.List(ctx, params)
	if err != nil {
		log.Printf("[EventService.ListEvents] repository error: %v", err)
		return nil, ErrInternal("get events failed")
	}
	totalPages := int((total + int64(params.Size) - 1) / int64(params.Size))
	return &EventListResult{List: events, Total: total, TotalPages: totalPages, Page: params.Page, Size: params.Size}, nil
}

func (s *EventService) ListTimeline(ctx context.Context, limit int) ([]models.Event, error) {
	events, err := s.repo.Event.ListTimeline(ctx, limit)
	if err != nil {
		log.Printf("[EventService.ListTimeline] repository error: %v", err)
		return nil, ErrInternal("get event timeline failed")
	}
	return events, nil
}

func (s *EventService) GetEvent(ctx context.Context, id int) (*models.Event, []models.Project, error) {
	event, err := s.repo.Event.GetByID(ctx, id)
	if err != nil {
		log.Printf("[EventService.GetEvent] repository error: %v", err)
		return nil, nil, ErrInternal("get event failed")
	}
	if event == nil {
		return nil, nil, ErrNotFound("event not found")
	}
	projectIDs, err := s.repo.Event.ListProjectIDs(ctx, id)
	if err != nil {
		return nil, nil, ErrInternal("get event projects failed")
	}
	projects := make([]models.Project, 0, len(projectIDs))
	for _, projectID := range projectIDs {
		project, err := s.repo.Project.GetByID(ctx, projectID)
		if err != nil {
			return nil, nil, ErrInternal("get event projects failed")
		}
		if project != nil && project.Status == models.ProjectStatusApproved {
			projects = append(projects, *project)
		}
	}
	return event, projects, nil
}

func (s *EventService) CreateEvent(ctx context.Context, event *models.Event) (*models.Event, error) {
	event.Name = strings.TrimSpace(event.Name)
	if event.Name == "" {
		return nil, ErrBadRequest("event name is required")
	}
	if len([]rune(event.Name)) > 200 {
		return nil, ErrBadRequest("event name is too long")
	}
	if err := s.repo.Event.Create(ctx, event); err != nil {
		log.Printf("[EventService.CreateEvent] repository error: %v", err)
		return nil, ErrInternal("create event failed")
	}
	return s.repo.Event.GetByID(ctx, event.ID)
}

func (s *EventService) UpdateEvent(ctx context.Context, event *models.Event) (*models.Event, error) {
	event.Name = strings.TrimSpace(event.Name)
	if event.Name == "" {
		return nil, ErrBadRequest("event name is required")
	}
	if err := s.repo.Event.Update(ctx, event); err != nil {
		log.Printf("[EventService.UpdateEvent] repository error: %v", err)
		return nil, ErrInternal("update event failed")
	}
	return s.repo.Event.GetByID(ctx, event.ID)
}

func (s *EventService) DeleteEvent(ctx context.Context, id int) error {
	if err := s.repo.Event.Delete(ctx, id); err != nil {
		log.Printf("[EventService.DeleteEvent] repository error: %v", err)
		return ErrInternal("delete event failed")
	}
	return nil
}
