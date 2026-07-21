package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	adminauth "github.com/kuaizu-team/kuaizu-service/internal/admin/auth"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/labstack/echo/v4"
)

func TestAdminJWTAuthRejectsLegacyTokenWithoutRole(t *testing.T) {
	authConfig := &adminauth.AdminConfig{Secret: "test-secret", Issuer: "test", ExpireHour: 1}
	config := &AdminJWTConfig{AuthConfig: authConfig}
	token, _, err := adminauth.GenerateAdminToken(authConfig, 1, "legacy-admin", 0, 0)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/stats", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	ctx := e.NewContext(req, recorder)
	handler := AdminJWTAuth(config)(func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	})

	err = handler(ctx)
	httpErr, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("error = %T %v, want *echo.HTTPError", err, err)
	}
	if httpErr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", httpErr.Code, http.StatusUnauthorized)
	}
}

type stubAdminAuthStateStore struct {
	admin *models.AdminUser
	err   error
}

func (s stubAdminAuthStateStore) GetAuthStateByID(context.Context, int) (*models.AdminUser, error) {
	return s.admin, s.err
}

func adminTokenForTest(t *testing.T, config *adminauth.AdminConfig, role, schoolID int) string {
	t.Helper()
	token, _, err := adminauth.GenerateAdminToken(config, 1, "admin", role, schoolID)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return token
}

func TestAdminJWTAuthUsesCurrentDatabaseRoleAndSchool(t *testing.T) {
	authConfig := &adminauth.AdminConfig{Secret: "test-secret", Issuer: "test", ExpireHour: 1}
	schoolID := 42
	config := &AdminJWTConfig{
		AuthConfig: authConfig,
		AdminUsers: stubAdminAuthStateStore{admin: &models.AdminUser{
			ID: 1, Username: "admin", Role: models.AdminRoleSchoolAdmin,
			SchoolID: &schoolID, Status: models.AdminUserStatusEnabled,
		}},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/stats", nil)
	req.Header.Set("Authorization", "Bearer "+adminTokenForTest(t, authConfig, models.AdminRoleSuperAdmin, 0))
	ctx := e.NewContext(req, httptest.NewRecorder())
	handler := AdminJWTAuth(config)(func(c echo.Context) error {
		if got := c.Get("adminRole"); got != models.AdminRoleSchoolAdmin {
			t.Fatalf("adminRole = %v, want %d", got, models.AdminRoleSchoolAdmin)
		}
		if got := c.Get("adminSchoolID"); got != schoolID {
			t.Fatalf("adminSchoolID = %v, want %d", got, schoolID)
		}
		return c.NoContent(http.StatusNoContent)
	})

	if err := handler(ctx); err != nil {
		t.Fatalf("handler error: %v", err)
	}
}

func TestAdminJWTAuthRejectsDisabledAdministrator(t *testing.T) {
	authConfig := &adminauth.AdminConfig{Secret: "test-secret", Issuer: "test", ExpireHour: 1}
	config := &AdminJWTConfig{
		AuthConfig: authConfig,
		AdminUsers: stubAdminAuthStateStore{admin: &models.AdminUser{
			ID: 1, Username: "admin", Role: models.AdminRoleSuperAdmin,
			Status: models.AdminUserStatusDisabled,
		}},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/stats", nil)
	req.Header.Set("Authorization", "Bearer "+adminTokenForTest(t, authConfig, models.AdminRoleSuperAdmin, 0))
	err := AdminJWTAuth(config)(func(echo.Context) error {
		t.Fatal("disabled administrator reached protected handler")
		return nil
	})(e.NewContext(req, httptest.NewRecorder()))

	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusUnauthorized {
		t.Fatalf("error = %T %v, want HTTP %d", err, err, http.StatusUnauthorized)
	}
}

func TestAdminJWTAuthPreservesSessionOnAuthorizationStoreFailure(t *testing.T) {
	authConfig := &adminauth.AdminConfig{Secret: "test-secret", Issuer: "test", ExpireHour: 1}
	config := &AdminJWTConfig{
		AuthConfig: authConfig,
		AdminUsers: stubAdminAuthStateStore{err: errors.New("database unavailable")},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/stats", nil)
	req.Header.Set("Authorization", "Bearer "+adminTokenForTest(t, authConfig, models.AdminRoleSuperAdmin, 0))
	err := AdminJWTAuth(config)(func(echo.Context) error {
		t.Fatal("request reached handler while authorization store was unavailable")
		return nil
	})(e.NewContext(req, httptest.NewRecorder()))

	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusServiceUnavailable {
		t.Fatalf("error = %T %v, want HTTP %d", err, err, http.StatusServiceUnavailable)
	}
}

func TestAdminJWTAuthRevokesOldSuperAdminRouteAfterDowngrade(t *testing.T) {
	authConfig := &adminauth.AdminConfig{Secret: "test-secret", Issuer: "test", ExpireHour: 1}
	config := &AdminJWTConfig{
		AuthConfig: authConfig,
		AdminUsers: stubAdminAuthStateStore{admin: &models.AdminUser{
			ID: 1, Username: "admin", Role: models.AdminRoleEventManager,
			Status: models.AdminUserStatusEnabled,
		}},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/admin/admins", nil)
	req.Header.Set("Authorization", "Bearer "+adminTokenForTest(t, authConfig, models.AdminRoleSuperAdmin, 0))
	err := AdminJWTAuth(config)(func(echo.Context) error {
		t.Fatal("downgraded administrator retained the old super-admin route")
		return nil
	})(e.NewContext(req, httptest.NewRecorder()))

	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusForbidden {
		t.Fatalf("error = %T %v, want HTTP %d", err, err, http.StatusForbidden)
	}
}
