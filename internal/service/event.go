package service

import (
	"context"
	"log"
	"strings"

	"github.com/jmoiron/sqlx"
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
	approved := models.ProjectStatusApproved
	const projectPageSize = 1000
	projects := make([]models.Project, 0)
	for page := 1; ; page++ {
		batch, total, listErr := s.repo.Project.List(ctx, repository.ListParams{
			Page: page, Size: projectPageSize, EventID: &id, Status: &approved,
		})
		if listErr != nil {
			return nil, nil, ErrInternal("get event projects failed")
		}
		projects = append(projects, batch...)
		if len(batch) == 0 || len(projects) >= int(total) {
			break
		}
	}
	return event, projects, nil
}

func validateEvent(event *models.Event) error {
	event.Name = strings.TrimSpace(event.Name)
	if event.Name == "" {
		return ErrBadRequest("event name is required")
	}
	if len([]rune(event.Name)) > 200 {
		return ErrBadRequest("event name is too long")
	}
	return nil
}

func (s *EventService) CreateEvent(ctx context.Context, event *models.Event) (*models.Event, error) {
	if err := validateEvent(event); err != nil {
		return nil, err
	}
	if err := s.repo.Event.Create(ctx, event); err != nil {
		log.Printf("[EventService.CreateEvent] repository error: %v", err)
		return nil, ErrInternal("create event failed")
	}
	return s.repo.Event.GetByID(ctx, event.ID)
}

// CreateEventTx validates and creates an event in the caller's transaction.
func (s *EventService) CreateEventTx(ctx context.Context, tx *sqlx.Tx, event *models.Event) error {
	if err := validateEvent(event); err != nil {
		return err
	}
	if err := repository.CreateEventTx(ctx, tx, event); err != nil {
		log.Printf("[EventService.CreateEventTx] repository error: %v", err)
		return ErrInternal("create event failed")
	}
	return nil
}

func (s *EventService) UpdateEvent(ctx context.Context, event *models.Event) (*models.Event, error) {
	if err := validateEvent(event); err != nil {
		return nil, err
	}
	if err := s.repo.Event.Update(ctx, event); err != nil {
		log.Printf("[EventService.UpdateEvent] repository error: %v", err)
		return nil, ErrInternal("update event failed")
	}
	return s.repo.Event.GetByID(ctx, event.ID)
}

// UpdateEventTx validates and updates an event in the caller's transaction.
func (s *EventService) UpdateEventTx(ctx context.Context, tx *sqlx.Tx, event *models.Event) error {
	if err := validateEvent(event); err != nil {
		return err
	}
	if err := repository.UpdateEventTx(ctx, tx, event); err != nil {
		log.Printf("[EventService.UpdateEventTx] repository error: %v", err)
		return ErrInternal("update event failed")
	}
	return nil
}
func (s *EventService) DeleteEvent(ctx context.Context, id int) error {
	if err := s.repo.Event.Delete(ctx, id); err != nil {
		log.Printf("[EventService.DeleteEvent] repository error: %v", err)
		return ErrInternal("delete event failed")
	}
	return nil
}

func (s *EventService) MergeEvent(ctx context.Context, sourceID, targetID int) (*models.Event, error) {
	if sourceID <= 0 || targetID <= 0 {
		return nil, ErrBadRequest("invalid event id")
	}
	if sourceID == targetID {
		return nil, ErrBadRequest("source event and target event must be different")
	}
	source, err := s.repo.Event.GetByID(ctx, sourceID)
	if err != nil {
		log.Printf("[EventService.MergeEvent] repository error getting source event: %v", err)
		return nil, ErrInternal("get source event failed")
	}
	if source == nil {
		return nil, ErrNotFound("source event not found")
	}
	target, err := s.repo.Event.GetByID(ctx, targetID)
	if err != nil {
		log.Printf("[EventService.MergeEvent] repository error getting target event: %v", err)
		return nil, ErrInternal("get target event failed")
	}
	if target == nil {
		return nil, ErrNotFound("target event not found")
	}
	if err := s.repo.Event.Merge(ctx, sourceID, targetID); err != nil {
		log.Printf("[EventService.MergeEvent] repository error: %v", err)
		return nil, ErrInternal("merge event failed")
	}
	return s.repo.Event.GetByID(ctx, targetID)
}
