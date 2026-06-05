package service

import (
	"context"
	"log"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

type InteractionService struct{ repo *repository.Repository }

func NewInteractionService(repo *repository.Repository) *InteractionService {
	return &InteractionService{repo: repo}
}

func (s *InteractionService) ensureTarget(ctx context.Context, target string, id int) error {
	if id <= 0 {
		return ErrBadRequest("invalid target id")
	}
	switch target {
	case repository.InteractionProject:
		item, err := s.repo.Project.GetByID(ctx, id)
		if err != nil {
			return ErrInternal("get project failed")
		}
		if item == nil {
			return ErrNotFound("project not found")
		}
	case repository.InteractionTalent:
		item, err := s.repo.TalentProfile.GetByID(ctx, id)
		if err != nil {
			return ErrInternal("get talent profile failed")
		}
		if item == nil {
			return ErrNotFound("talent profile not found")
		}
	default:
		return ErrBadRequest("invalid target")
	}
	return nil
}

func (s *InteractionService) Get(ctx context.Context, target string, id, userID int) (*models.Interaction, error) {
	if err := s.ensureTarget(ctx, target, id); err != nil {
		return nil, err
	}
	result, err := s.repo.Interaction.Get(ctx, target, id, userID)
	if err != nil {
		log.Printf("[Interaction.Get] %v", err)
		return nil, ErrInternal("get interactions failed")
	}
	return result, nil
}

func (s *InteractionService) Toggle(ctx context.Context, target, kind string, id, userID int) (*models.Interaction, error) {
	if err := s.ensureTarget(ctx, target, id); err != nil {
		return nil, err
	}
	result, err := s.repo.Interaction.Toggle(ctx, target, kind, id, userID)
	if err != nil {
		log.Printf("[Interaction.Toggle] %v", err)
		return nil, ErrInternal("toggle interaction failed")
	}
	return result, nil
}

func (s *InteractionService) Share(ctx context.Context, target string, id, userID int, channel string) (*models.Interaction, error) {
	if channel != "wechat" && channel != "timeline" && channel != "copy" {
		return nil, ErrBadRequest("invalid share channel")
	}
	if err := s.ensureTarget(ctx, target, id); err != nil {
		return nil, err
	}
	result, err := s.repo.Interaction.Share(ctx, target, id, userID, channel)
	if err != nil {
		log.Printf("[Interaction.Share] %v", err)
		return nil, ErrInternal("record share failed")
	}
	return result, nil
}

func (s *InteractionService) ListUsers(ctx context.Context, target, kind string, id, page, size, days int) (map[string]interface{}, error) {
	if kind != "like" && kind != "favorite" && kind != "share" {
		return nil, ErrBadRequest("invalid interaction type")
	}
	if err := s.ensureTarget(ctx, target, id); err != nil {
		return nil, err
	}
	page, size = normalizePageParams(page, size)
	if days <= 0 || days > 30 {
		days = 30
	}
	if page*size > 1000 {
		return nil, ErrBadRequest("interaction detail is limited to the latest 1000 records")
	}
	list, total, err := s.repo.Interaction.ListUsers(ctx, target, kind, id, page, size, days)
	if err != nil {
		return nil, ErrInternal("list interaction users failed")
	}
	return map[string]interface{}{"list": list, "page": page, "size": size, "total": total}, nil
}
