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
	pendingRepo := &fakePendingInvitationRepo{}
	svc := NewInvitationFeedbackService(&repository.Repository{
		InvitationFeedback: repo,
		PendingInvitation:  pendingRepo,
	})
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
	if pendingRepo.clearUserID != 1001 {
		t.Fatalf("clear pending user_id = %d, want 1001", pendingRepo.clearUserID)
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

func TestInvitationFeedbackCreateAndGetPendingInvitation(t *testing.T) {
	pendingRepo := &fakePendingInvitationRepo{}
	svc := NewInvitationFeedbackService(&repository.Repository{PendingInvitation: pendingRepo})

	if err := svc.CreatePendingSuperAdminInvitation(context.Background(), 1001); err != nil {
		t.Fatalf("CreatePendingSuperAdminInvitation returned error: %v", err)
	}
	if pendingRepo.userID != 1001 || pendingRepo.inviteType != models.PendingInvitationTypeSuperAdmin {
		t.Fatalf("pending invitation = (%d, %s)", pendingRepo.userID, pendingRepo.inviteType)
	}

	got, err := svc.GetPendingInvitation(context.Background(), 1001)
	if err != nil {
		t.Fatalf("GetPendingInvitation returned error: %v", err)
	}
	if got == nil || got.InviteType != models.PendingInvitationTypeSuperAdmin {
		t.Fatalf("pending invitation = %#v", got)
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

type fakePendingInvitationRepo struct {
	repository.PendingInvitationRepo

	userID      int
	inviteType  string
	expireAt    time.Time
	clearUserID int
}

func (f *fakePendingInvitationRepo) Upsert(_ context.Context, userID int, inviteType string, expireAt time.Time) error {
	f.userID = userID
	f.inviteType = inviteType
	f.expireAt = expireAt
	return nil
}

func (f *fakePendingInvitationRepo) GetActiveByUserID(_ context.Context, userID int, _ time.Time) (*models.PendingInvitation, error) {
	return &models.PendingInvitation{
		UserID:     userID,
		InviteType: models.PendingInvitationTypeSuperAdmin,
		ExpireAt:   f.expireAt,
	}, nil
}

func (f *fakePendingInvitationRepo) ClearByUserID(_ context.Context, userID int) error {
	f.clearUserID = userID
	return nil
}
