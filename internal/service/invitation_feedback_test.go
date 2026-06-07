package service

import (
	"context"
	"testing"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

func TestInvitationFeedbackSubmitInterestedTrimsAndUpserts(t *testing.T) {
	repo := &fakeInvitationFeedbackRepo{}
	svc := NewInvitationFeedbackService(&repository.Repository{InvitationFeedback: repo})

	text := "  想了解校区运营权限  "
	got, err := svc.SubmitFeedback(context.Background(), 1001, models.InvitationFeedbackStatusInterested, &text)
	if err != nil {
		t.Fatalf("SubmitFeedback returned error: %v", err)
	}
	if repo.upsertUserID != 1001 {
		t.Fatalf("user_id = %d, want 1001", repo.upsertUserID)
	}
	if repo.upsertStatus != models.InvitationFeedbackStatusInterested {
		t.Fatalf("status = %s", repo.upsertStatus)
	}
	if repo.upsertText == nil || *repo.upsertText != "想了解校区运营权限" {
		t.Fatalf("intention_text = %v", repo.upsertText)
	}
	if got.ConversationStatus != nil {
		t.Fatalf("conversation_status = %v, want nil", got.ConversationStatus)
	}
}

func TestInvitationFeedbackSubmitInterestedRequiresText(t *testing.T) {
	svc := NewInvitationFeedbackService(&repository.Repository{InvitationFeedback: &fakeInvitationFeedbackRepo{}})
	text := "   "

	if _, err := svc.SubmitFeedback(context.Background(), 1001, models.InvitationFeedbackStatusInterested, &text); err == nil {
		t.Fatal("SubmitFeedback returned nil error, want validation error")
	}
}

func TestInvitationFeedbackSubmitNotInterestedClearsText(t *testing.T) {
	repo := &fakeInvitationFeedbackRepo{}
	svc := NewInvitationFeedbackService(&repository.Repository{InvitationFeedback: repo})
	text := "不用了"

	got, err := svc.SubmitFeedback(context.Background(), 1001, models.InvitationFeedbackStatusNotInterested, &text)
	if err != nil {
		t.Fatalf("SubmitFeedback returned error: %v", err)
	}
	if repo.upsertStatus != models.InvitationFeedbackStatusNotInterested {
		t.Fatalf("status = %s", repo.upsertStatus)
	}
	if repo.upsertText != nil || got.IntentionText != nil {
		t.Fatalf("intention_text was not cleared")
	}
}

func TestInvitationFeedbackSetConversationCreatesPendingWhenMissing(t *testing.T) {
	repo := &fakeInvitationFeedbackRepo{}
	svc := NewInvitationFeedbackService(&repository.Repository{InvitationFeedback: repo})

	got, err := svc.SetConversationStatus(context.Background(), 1001, models.InvitationConversationStatusAccepted)
	if err != nil {
		t.Fatalf("SetConversationStatus returned error: %v", err)
	}
	if repo.conversationUserID != 1001 {
		t.Fatalf("user_id = %d, want 1001", repo.conversationUserID)
	}
	if got.Status != models.InvitationFeedbackStatusPending {
		t.Fatalf("status = %s, want pending", got.Status)
	}
	if got.ConversationStatus == nil || *got.ConversationStatus != models.InvitationConversationStatusAccepted {
		t.Fatalf("conversation_status = %v", got.ConversationStatus)
	}
}

type fakeInvitationFeedbackRepo struct {
	repository.InvitationFeedbackRepo

	upsertUserID       int
	upsertStatus       string
	upsertText         *string
	resetUserID        int
	conversationUserID int
	conversationStatus string
}

func (f *fakeInvitationFeedbackRepo) GetByUserID(_ context.Context, userID int) (*models.InvitationFeedback, error) {
	return nil, nil
}

func (f *fakeInvitationFeedbackRepo) UpsertFeedback(_ context.Context, userID int, status string, intentionText *string) (*models.InvitationFeedback, error) {
	f.upsertUserID = userID
	f.upsertStatus = status
	f.upsertText = intentionText
	now := time.Now()
	return &models.InvitationFeedback{
		UserID:        userID,
		Status:        status,
		IntentionText: intentionText,
		UpdatedAt:     &now,
	}, nil
}

func (f *fakeInvitationFeedbackRepo) ResetAfterInviteSent(_ context.Context, userID int) (*models.InvitationFeedback, error) {
	f.resetUserID = userID
	now := time.Now()
	return &models.InvitationFeedback{
		UserID:    userID,
		Status:    models.InvitationFeedbackStatusPending,
		UpdatedAt: &now,
	}, nil
}

func (f *fakeInvitationFeedbackRepo) UpsertConversationStatus(_ context.Context, userID int, conversationStatus string) (*models.InvitationFeedback, error) {
	f.conversationUserID = userID
	f.conversationStatus = conversationStatus
	now := time.Now()
	return &models.InvitationFeedback{
		UserID:             userID,
		Status:             models.InvitationFeedbackStatusPending,
		ConversationStatus: &conversationStatus,
		UpdatedAt:          &now,
	}, nil
}
