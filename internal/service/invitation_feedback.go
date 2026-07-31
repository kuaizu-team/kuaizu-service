package service

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

// InvitationFeedbackService handles super-admin invitation feedback workflows.
type InvitationFeedbackService struct {
	repo *repository.Repository
}

const pendingInvitationTTL = 7 * 24 * time.Hour

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
	if s.repo.PendingInvitation != nil {
		if err := s.repo.PendingInvitation.ClearByUserID(ctx, userID); err != nil {
			log.Printf("[InvitationFeedbackService.SubmitFeedback] clear pending invitation error: %v", err)
			return nil, ErrInternal("clear pending invitation failed")
		}
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
	// "in_progress" is only valid while the matching display invitation is
	// active. Once its seven-day TTL expires, expose the conversation as
	// released so the admin UI no longer keeps showing "正在聊".
	if f.ConversationStatus != nil &&
		*f.ConversationStatus == models.InvitationConversationStatusInProgress &&
		s.repo.PendingInvitation != nil {
		pending, err := s.repo.PendingInvitation.GetActiveByUserID(ctx, userID, time.Now())
		if err != nil {
			log.Printf("[InvitationFeedbackService.GetStatus] pending invitation repository error: %v", err)
			return nil, ErrInternal("get pending invitation failed")
		}
		if pending == nil {
			f.ConversationStatus = nil
		}
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
	if normalized == models.InvitationConversationStatusInProgress {
		if err := s.CreatePendingSuperAdminInvitation(ctx, userID); err != nil {
			return nil, err
		}
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

func (s *InvitationFeedbackService) CreatePendingSuperAdminInvitation(ctx context.Context, userID int) error {
	if userID <= 0 {
		return ErrBadRequest("invalid user_id")
	}
	if s.repo.PendingInvitation == nil {
		return ErrInternal("pending invitation repository is nil")
	}
	expireAt := time.Now().Add(pendingInvitationTTL)
	if err := s.repo.PendingInvitation.Upsert(ctx, userID, models.PendingInvitationTypeSuperAdmin, expireAt); err != nil {
		log.Printf("[InvitationFeedbackService.CreatePendingSuperAdminInvitation] repository error: %v", err)
		return ErrInternal("create pending invitation failed")
	}
	return nil
}

func (s *InvitationFeedbackService) GetPendingInvitation(ctx context.Context, userID int) (*models.PendingInvitation, error) {
	if userID <= 0 {
		return nil, ErrBadRequest("invalid user_id")
	}
	if s.repo.PendingInvitation == nil {
		return nil, ErrInternal("pending invitation repository is nil")
	}
	if s.repo.InvitationFeedback != nil {
		feedback, err := s.repo.InvitationFeedback.GetByUserID(ctx, userID)
		if err != nil {
			log.Printf("[InvitationFeedbackService.GetPendingInvitation] feedback repository error: %v", err)
			return nil, ErrInternal("get invitation feedback failed")
		}
		if feedback != nil && feedback.Status != models.InvitationFeedbackStatusPending {
			if err := s.repo.PendingInvitation.ClearByUserID(ctx, userID); err != nil {
				log.Printf("[InvitationFeedbackService.GetPendingInvitation] clear responded pending invitation error: %v", err)
				return nil, ErrInternal("clear pending invitation failed")
			}
			return nil, nil
		}
	}
	item, err := s.repo.PendingInvitation.GetActiveByUserID(ctx, userID, time.Now())
	if err != nil {
		log.Printf("[InvitationFeedbackService.GetPendingInvitation] repository error: %v", err)
		return nil, ErrInternal("get pending invitation failed")
	}
	return item, nil
}

func (s *InvitationFeedbackService) ClearPendingInvitation(ctx context.Context, userID int) error {
	if userID <= 0 {
		return ErrBadRequest("invalid user_id")
	}
	if s.repo.PendingInvitation == nil {
		return ErrInternal("pending invitation repository is nil")
	}
	if err := s.repo.PendingInvitation.ClearByUserID(ctx, userID); err != nil {
		log.Printf("[InvitationFeedbackService.ClearPendingInvitation] repository error: %v", err)
		return ErrInternal("clear pending invitation failed")
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
