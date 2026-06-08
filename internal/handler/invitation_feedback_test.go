package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/service"
	"github.com/labstack/echo/v4"
)

func TestSubmitInvitationFeedbackUsesCurrentUserID(t *testing.T) {
	repo := &fakeHandlerInvitationFeedbackRepo{}
	server := &Server{
		repo: &repository.Repository{InvitationFeedback: repo},
		svc: &service.Services{
			Invitation: service.NewInvitationFeedbackService(&repository.Repository{InvitationFeedback: repo}),
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v2/invitation/feedback", strings.NewReader(`{
		"user_id": 9999,
		"status": "interested",
		"intention_text": "想了解"
	}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	ctx := echo.New().NewContext(req, rec)
	ctx.Set("userID", 1001)

	if err := server.SubmitInvitationFeedback(ctx); err != nil {
		t.Fatalf("SubmitInvitationFeedback returned error: %v", err)
	}
	if repo.userID != 1001 {
		t.Fatalf("saved user_id = %d, want current login user 1001", repo.userID)
	}
}

func TestGetMyPendingInvitationReturnsPendingVO(t *testing.T) {
	pendingRepo := &fakeHandlerPendingInvitationRepo{}
	repo := &repository.Repository{PendingInvitation: pendingRepo}
	server := &Server{
		repo: repo,
		svc: &service.Services{
			Invitation: service.NewInvitationFeedbackService(repo),
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v2/users/me/pending-invitation", nil)
	rec := httptest.NewRecorder()
	ctx := echo.New().NewContext(req, rec)
	ctx.Set("userID", 1001)

	if err := server.GetMyPendingInvitation(ctx); err != nil {
		t.Fatalf("GetMyPendingInvitation returned error: %v", err)
	}

	var body struct {
		Data *pendingInvitationVO `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data == nil || !body.Data.HasPending || body.Data.Type != models.PendingInvitationTypeSuperAdmin {
		t.Fatalf("data = %#v", body.Data)
	}
}

type fakeHandlerInvitationFeedbackRepo struct {
	repository.InvitationFeedbackRepo
	userID int
}

func (f *fakeHandlerInvitationFeedbackRepo) UpsertFeedback(_ context.Context, userID int, status string, intentionText *string) (*models.InvitationFeedback, error) {
	f.userID = userID
	now := time.Now()
	return &models.InvitationFeedback{
		UserID:        userID,
		Status:        status,
		IntentionText: intentionText,
		UpdatedAt:     &now,
	}, nil
}

type fakeHandlerPendingInvitationRepo struct {
	repository.PendingInvitationRepo
}

func (f *fakeHandlerPendingInvitationRepo) GetActiveByUserID(_ context.Context, userID int, _ time.Time) (*models.PendingInvitation, error) {
	return &models.PendingInvitation{
		UserID:     userID,
		InviteType: models.PendingInvitationTypeSuperAdmin,
		ExpireAt:   time.Now().Add(time.Hour),
	}, nil
}
