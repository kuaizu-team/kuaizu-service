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

func TestSendAdminSmsMarksInvitationInProgressAfterSuccessfulInvite(t *testing.T) {
	resetRepo := &fakeAdminSmsInvitationFeedbackRepo{}
	pendingRepo := &fakeAdminSmsPendingInvitationRepo{}
	server := newAdminSmsSendServer(t, http.StatusOK, responseEnvelope(map[string]interface{}{
		"success":      true,
		"template_key": "INVITE_SUPER_ADMIN",
		"user_id":      1001,
		"record_id":    88,
	}), resetRepo, pendingRepo)

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
	if resetRepo.conversationUserID != 1001 {
		t.Fatalf("conversation user_id = %d, want 1001", resetRepo.conversationUserID)
	}
	if resetRepo.conversationStatus != models.InvitationConversationStatusInProgress {
		t.Fatalf("conversation status = %s, want in_progress", resetRepo.conversationStatus)
	}
	if pendingRepo.userID != 1001 || pendingRepo.inviteType != models.PendingInvitationTypeSuperAdmin {
		t.Fatalf("pending invitation = (%d, %s), want (1001, SUPER_ADMIN)", pendingRepo.userID, pendingRepo.inviteType)
	}
}

func TestSendAdminSmsDoesNotResetInvitationOnSendFailure(t *testing.T) {
	resetRepo := &fakeAdminSmsInvitationFeedbackRepo{}
	pendingRepo := &fakeAdminSmsPendingInvitationRepo{}
	server := newAdminSmsSendServer(t, http.StatusInternalServerError, `{"code":500,"message":"failed"}`, resetRepo, pendingRepo)

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
	if resetRepo.conversationUserID != 0 {
		t.Fatalf("conversation user_id = %d, want 0", resetRepo.conversationUserID)
	}
	if pendingRepo.userID != 0 {
		t.Fatalf("pending user_id = %d, want 0", pendingRepo.userID)
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

func newAdminSmsSendServer(t *testing.T, statusCode int, body string, resetRepo *fakeAdminSmsInvitationFeedbackRepo, pendingRepo *fakeAdminSmsPendingInvitationRepo) *AdminServer {
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
		PendingInvitation:  pendingRepo,
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
	conversationUserID int
	conversationStatus string
}

func (f *fakeAdminSmsInvitationFeedbackRepo) UpsertConversationStatus(_ context.Context, userID int, conversationStatus string) (*models.InvitationFeedback, error) {
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

func TestWithSuperAdminInvitationVariablesAddsMiniProgramPath(t *testing.T) {
	variables := withSuperAdminInvitationVariables(map[string]interface{}{"nickname": "张三"})

	if variables["nickname"] != "张三" {
		t.Fatalf("nickname was not preserved")
	}
	if variables["invite_path"] != "/pages/home/home?invitation_feedback=1&source=sms&invite_type=super_admin" {
		t.Fatalf("invite_path = %v", variables["invite_path"])
	}
	if variables["invite_query"] != "invitation_feedback=1&source=sms&invite_type=super_admin" {
		t.Fatalf("invite_query = %v", variables["invite_query"])
	}
}

type fakeAdminSmsPendingInvitationRepo struct {
	repository.PendingInvitationRepo
	userID     int
	inviteType string
	expireAt   time.Time
}

func (f *fakeAdminSmsPendingInvitationRepo) Upsert(_ context.Context, userID int, inviteType string, expireAt time.Time) error {
	f.userID = userID
	f.inviteType = inviteType
	f.expireAt = expireAt
	return nil
}

func intPtr(v int) *int {
	return &v
}
