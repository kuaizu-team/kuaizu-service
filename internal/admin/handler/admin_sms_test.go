package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

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

func newAdminSmsPermissionServer(user *models.User) *AdminServer {
	repo := &repository.Repository{User: fakeAdminSmsUserRepo{user: user}}
	return &AdminServer{
		repo: repo,
		svc: &service.Services{
			User: service.NewUserService(repo, nil),
		},
	}
}

type fakeAdminSmsUserRepo struct {
	repository.UserRepo

	user *models.User
}

func (f fakeAdminSmsUserRepo) GetByID(_ context.Context, _ int) (*models.User, error) {
	return f.user, nil
}

func intPtr(v int) *int {
	return &v
}
