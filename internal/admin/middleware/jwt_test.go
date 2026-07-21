package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	adminauth "github.com/kuaizu-team/kuaizu-service/internal/admin/auth"
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
