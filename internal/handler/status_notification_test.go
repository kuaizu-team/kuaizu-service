package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/service"
	"github.com/labstack/echo/v4"
)

func TestGetMyPendingStatusNotificationSuppressesLowerPriorityWhenInvitationPending(t *testing.T) {
	statusRepo := &fakeStatusNotificationRepo{}
	pendingRepo := &fakeHandlerPendingInvitationRepo{}
	repo := &repository.Repository{
		PendingInvitation:  pendingRepo,
		StatusNotification: statusRepo,
	}
	server := &Server{
		repo: repo,
		svc: &service.Services{
			Invitation: service.NewInvitationFeedbackService(repo),
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v2/users/me/status-notifications/pending", nil)
	rec := httptest.NewRecorder()
	ctx := echo.New().NewContext(req, rec)
	ctx.Set("userID", 1001)

	if err := server.GetMyPendingStatusNotification(ctx); err != nil {
		t.Fatalf("GetMyPendingStatusNotification returned error: %v", err)
	}
	if statusRepo.markAllUserID != 1001 {
		t.Fatalf("mark all user_id = %d, want 1001", statusRepo.markAllUserID)
	}
	if statusRepo.getPendingUserID != 0 {
		t.Fatalf("GetPending was called for user_id = %d, want 0", statusRepo.getPendingUserID)
	}
	var body struct {
		Data *models.StatusNotification `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data != nil {
		t.Fatalf("data = %#v, want nil", body.Data)
	}
}

type fakeStatusNotificationRepo struct {
	repository.StatusNotificationRepo

	getPendingUserID int
	markID           int64
	markUserID       int
	markAllUserID    int
}

func (f *fakeStatusNotificationRepo) GetPending(_ context.Context, userID int) (*models.StatusNotification, error) {
	f.getPendingUserID = userID
	return &models.StatusNotification{
		ID:       10,
		UserID:   userID,
		Type:     models.StatusNotificationApplicationAccepted,
		Priority: 100,
	}, nil
}

func (f *fakeStatusNotificationRepo) MarkDisplayed(_ context.Context, id int64, userID int) error {
	f.markID = id
	f.markUserID = userID
	return nil
}

func (f *fakeStatusNotificationRepo) MarkAllPendingDisplayed(_ context.Context, userID int) error {
	f.markAllUserID = userID
	return nil
}
