package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/messagecenter"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/service"
	"github.com/labstack/echo/v4"
)

func TestEnsureAdminCanAccessUserAllowsSameSchool(t *testing.T) {
	e := echo.New()
	ctx := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
	ctx.Set("adminSchoolID", 10)

	server := newAdminSmsPermissionServer(&models.User{ID: 1001, SchoolID: intPtr(10)})

	if err := server.ensureAdminCanAccessUser(ctx, 1001); err != nil {
		t.Fatalf("ensureAdminCanAccessUser returned error: %v", err)
	}
}

func TestEnsureAdminCanAccessUserRejectsDifferentSchool(t *testing.T) {
	e := echo.New()
	rec := httptest.NewRecorder()
	ctx := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	ctx.Set("adminSchoolID", 10)

	server := newAdminSmsPermissionServer(&models.User{ID: 1001, SchoolID: intPtr(20)})

	if err := server.ensureAdminCanAccessUser(ctx, 1001); err != nil {
		t.Fatalf("ensureAdminCanAccessUser returned error: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestEnsureAdminCanAccessUserAllowsUnscopedAdminWithoutLookup(t *testing.T) {
	e := echo.New()
	ctx := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())

	server := newAdminSmsPermissionServer(nil)

	if err := server.ensureAdminCanAccessUser(ctx, 1001); err != nil {
		t.Fatalf("ensureAdminCanAccessUser returned error: %v", err)
	}
}

func TestSendAdminSmsResetsInvitationAfterSuccessfulInvite(t *testing.T) {
	resetRepo := &fakeAdminSmsInvitationFeedbackRepo{}
	server := newAdminSmsSendServer(t, http.StatusOK, responseEnvelope(map[string]interface{}{
		"success":      true,
		"template_key": "INVITE_SUPER_ADMIN",
		"user_id":      1001,
		"record_id":    88,
	}), resetRepo)

	req := httptest.NewRequest(http.MethodPost, "/admin/sms/send", strings.NewReader(`{
		"template_key": "INVITE_SUPER_ADMIN",
		"user_id": 1001
	}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	ctx := echo.New().NewContext(req, rec)

	if err := server.SendAdminSms(ctx); err != nil {
		t.Fatalf("SendAdminSms returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if resetRepo.resetUserID != 1001 {
		t.Fatalf("reset user_id = %d, want 1001", resetRepo.resetUserID)
	}
}

func TestSendAdminSmsDoesNotResetInvitationOnSendFailure(t *testing.T) {
	resetRepo := &fakeAdminSmsInvitationFeedbackRepo{}
	server := newAdminSmsSendServer(t, http.StatusInternalServerError, `{"code":500,"message":"failed"}`, resetRepo)

	req := httptest.NewRequest(http.MethodPost, "/admin/sms/send", strings.NewReader(`{
		"template_key": "INVITE_SUPER_ADMIN",
		"user_id": 1001
	}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	ctx := echo.New().NewContext(req, rec)

	if err := server.SendAdminSms(ctx); err != nil {
		t.Fatalf("SendAdminSms returned error: %v", err)
	}
	if resetRepo.resetUserID != 0 {
		t.Fatalf("reset user_id = %d, want 0", resetRepo.resetUserID)
	}
}

func newAdminSmsPermissionServer(user *models.User) *AdminServer {
	repo := &repository.Repository{User: fakeAdminSmsUserRepo{user: user}}
	return &AdminServer{
		repo: repo,
		svc: &service.Services{
			User: service.NewUserService(repo, nil),
		},
	}
}

func newAdminSmsSendServer(t *testing.T, statusCode int, body string, resetRepo *fakeAdminSmsInvitationFeedbackRepo) *AdminServer {
	t.Helper()
	messageCenter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/admin/sms/send" {
			t.Fatalf("path = %s, want /api/v2/admin/sms/send", r.URL.Path)
		}
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(messageCenter.Close)

	repo := &repository.Repository{
		User:               fakeAdminSmsUserRepo{},
		InvitationFeedback: resetRepo,
	}
	return &AdminServer{
		repo: repo,
		svc: &service.Services{
			User:       service.NewUserService(repo, nil),
			AdminSms:   service.NewAdminSmsService(messagecenter.NewClient(messageCenter.URL, "test-token", time.Second), nil),
			Invitation: service.NewInvitationFeedbackService(repo),
		},
	}
}

func responseEnvelope(data map[string]interface{}) string {
	body, _ := json.Marshal(map[string]interface{}{
		"code":    200,
		"message": "success",
		"data":    data,
	})
	return string(body)
}

type fakeAdminSmsUserRepo struct {
	repository.UserRepo

	user *models.User
}

func (f fakeAdminSmsUserRepo) GetByID(_ context.Context, _ int) (*models.User, error) {
	return f.user, nil
}

type fakeAdminSmsInvitationFeedbackRepo struct {
	repository.InvitationFeedbackRepo
	resetUserID int
}

func (f *fakeAdminSmsInvitationFeedbackRepo) ResetAfterInviteSent(_ context.Context, userID int) (*models.InvitationFeedback, error) {
	f.resetUserID = userID
	now := time.Now()
	return &models.InvitationFeedback{
		UserID:    userID,
		Status:    models.InvitationFeedbackStatusPending,
		UpdatedAt: &now,
	}, nil
}

func intPtr(v int) *int {
	return &v
}
