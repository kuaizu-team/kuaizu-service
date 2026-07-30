package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/labstack/echo/v4"
)

func TestCanManageEvents(t *testing.T) {
	tests := []struct {
		name string
		role int
		want bool
	}{
		{name: "platform super admin", role: models.AdminRoleSuperAdmin, want: true},
		{name: "school super admin", role: models.AdminRoleSchoolSuperAdmin, want: true},
		{name: "school admin", role: models.AdminRoleSchoolAdmin, want: true},
		{name: "event manager", role: models.AdminRoleEventManager, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := canManageEvents(tt.role); got != tt.want {
				t.Fatalf("canManageEvents(%d) = %v, want %v", tt.role, got, tt.want)
			}
		})
	}
}

func TestCanMergeEvents(t *testing.T) {
	if !canMergeEvents(models.AdminRoleSuperAdmin) {
		t.Fatal("platform super administrator should be able to merge events")
	}
	if canMergeEvents(models.AdminRoleSchoolSuperAdmin) ||
		canMergeEvents(models.AdminRoleSchoolAdmin) ||
		canMergeEvents(models.AdminRoleEventManager) {
		t.Fatal("non-platform administrators must not merge events")
	}
}

func TestSchoolAdminEventDetailScopeAndManagerPermissions(t *testing.T) {
	schoolID, otherSchoolID := 22, 23
	schoolLevel, nationalLevel := "school", "national"
	e := echo.New()
	ctx := e.NewContext(httptest.NewRequest(http.MethodGet, "/admin/events/7", nil), httptest.NewRecorder())
	ctx.Set("adminRole", models.AdminRoleSchoolAdmin)
	ctx.Set("adminID", 3)
	ctx.Set("adminSchoolID", schoolID)
	server := &AdminServer{}

	ownSchoolEvent := &models.Event{ID: 7, Level: &schoolLevel, SchoolID: &schoolID}
	otherSchoolEvent := &models.Event{ID: 8, Level: &schoolLevel, SchoolID: &otherSchoolID}
	nationalEvent := &models.Event{ID: 9, Level: &nationalLevel}

	if !server.canViewEventInScope(ctx, ownSchoolEvent) || !server.canViewEventInScope(ctx, nationalEvent) {
		t.Fatal("school admin could not view own-school and national events")
	}
	if server.canViewEventInScope(ctx, otherSchoolEvent) {
		t.Fatal("school admin could view another school's event")
	}
	if !server.canViewEventManagerCredentials(ctx, ownSchoolEvent) ||
		server.canViewEventManagerCredentials(ctx, nationalEvent) {
		t.Fatal("school admin manager credential scope is incorrect")
	}
	if !server.canCreateEventManager(ctx, ownSchoolEvent) || server.canEditEventManager(ctx, ownSchoolEvent) {
		t.Fatal("school admin should create but not edit an own-school event manager")
	}
}
