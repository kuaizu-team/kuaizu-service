package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/labstack/echo/v4"
)

func TestUpdateAdminCommissionRateRequiresSuperAdmin(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/admin/admins/2/commission-rate", strings.NewReader(`{"commissionRate":25}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.SetParamNames("id")
	ctx.SetParamValues("2")
	ctx.Set("adminRole", models.AdminRoleSchoolSuperAdmin)

	server := &AdminServer{}

	if err := server.UpdateAdminCommissionRate(ctx); err != nil {
		t.Fatalf("UpdateAdminCommissionRate returned error: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}
