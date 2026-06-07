package service

import (
	"context"
	"log"
	"strings"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

// InvitationFeedbackService handles super-admin invitation feedback workflows.
type InvitationFeedbackService struct {
	repo *repository.Repository
}

func NewInvitationFeedbackService(repo *repository.Repository) *InvitationFeedbackService {
	return &InvitationFeedbackService{repo: repo}
}

func (s *InvitationFeedbackService) SubmitFeedback(ctx context.Context, userID int, status string, intentionText *string) (*models.InvitationFeedback, error) {
	if userID <= 0 {
		return nil, ErrBadRequest("invalid user_id")
	}

	normalizedStatus := strings.TrimSpace(status)
	var normalizedText *string
	switch normalizedStatus {
	case models.InvitationFeedbackStatusInterested:
		text := ""
		if intentionText != nil {
			text = strings.TrimSpace(*intentionText)
		}
		if text == "" {
			return nil, ErrBadRequest("intention_text is required")
		}
		if len([]rune(text)) > 500 {
			return nil, ErrBadRequest("intention_text must be 1-500 characters")
		}
		normalizedText = &text
	case models.InvitationFeedbackStatusNotInterested:
		normalizedText = nil
	default:
		return nil, ErrBadRequest("invalid status")
	}

	f, err := s.repo.InvitationFeedback.UpsertFeedback(ctx, userID, normalizedStatus, normalizedText)
	if err != nil {
		log.Printf("[InvitationFeedbackService.SubmitFeedback] repository error: %v", err)
		return nil, ErrInternal("save invitation feedback failed")
	}
	return f, nil
}

func (s *InvitationFeedbackService) GetStatus(ctx context.Context, userID int) (*models.InvitationFeedback, error) {
	if userID <= 0 {
		return nil, ErrBadRequest("invalid user_id")
	}

	f, err := s.repo.InvitationFeedback.GetByUserID(ctx, userID)
	if err != nil {
		log.Printf("[InvitationFeedbackService.GetStatus] repository error: %v", err)
		return nil, ErrInternal("get invitation feedback failed")
	}
	if f == nil {
		return &models.InvitationFeedback{
			UserID: userID,
			Status: models.InvitationFeedbackStatusPending,
		}, nil
	}
	return f, nil
}

func (s *InvitationFeedbackService) SetConversationStatus(ctx context.Context, userID int, conversationStatus string) (*models.InvitationFeedback, error) {
	if userID <= 0 {
		return nil, ErrBadRequest("invalid user_id")
	}
	normalized := strings.TrimSpace(conversationStatus)
	if !validInvitationConversationStatus(normalized) {
		return nil, ErrBadRequest("invalid conversation_status")
	}

	f, err := s.repo.InvitationFeedback.UpsertConversationStatus(ctx, userID, normalized)
	if err != nil {
		log.Printf("[InvitationFeedbackService.SetConversationStatus] repository error: %v", err)
		return nil, ErrInternal("update invitation conversation status failed")
	}
	return f, nil
}

func (s *InvitationFeedbackService) ResetAfterInviteSent(ctx context.Context, userID int) error {
	if userID <= 0 {
		return ErrBadRequest("invalid user_id")
	}
	if _, err := s.repo.InvitationFeedback.ResetAfterInviteSent(ctx, userID); err != nil {
		log.Printf("[InvitationFeedbackService.ResetAfterInviteSent] repository error: %v", err)
		return ErrInternal("reset invitation feedback failed")
	}
	return nil
}

func validInvitationConversationStatus(status string) bool {
	switch status {
	case models.InvitationConversationStatusInProgress,
		models.InvitationConversationStatusAccepted,
		models.InvitationConversationStatusRejected:
		return true
	default:
		return false
	}
}
