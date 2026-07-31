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

func TestDeleteAdminRejectsSelfBeforeRepositoryAccess(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/admin/admins/7", nil)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.SetParamNames("id")
	ctx.SetParamValues("7")
	ctx.Set("adminID", 7)
	ctx.Set("adminRole", models.AdminRoleSuperAdmin)

	server := &AdminServer{}
	if err := server.DeleteAdmin(ctx); err != nil {
		t.Fatalf("DeleteAdmin returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCanViewInvitationFeedback(t *testing.T) {
	tests := []struct {
		name string
		role int
		want bool
	}{
		{name: "platform super admin", role: models.AdminRoleSuperAdmin, want: true},
		{name: "school super admin", role: models.AdminRoleSchoolSuperAdmin, want: true},
		{name: "school admin", role: models.AdminRoleSchoolAdmin, want: false},
		{name: "event manager", role: models.AdminRoleEventManager, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := canViewInvitationFeedback(tt.role); got != tt.want {
				t.Fatalf("canViewInvitationFeedback(%d) = %v, want %v", tt.role, got, tt.want)
			}
		})
	}
}
