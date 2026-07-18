package handler

import (
	"testing"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

func TestCanManageEvents(t *testing.T) {
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
			if got := canManageEvents(tt.role); got != tt.want {
				t.Fatalf("canManageEvents(%d) = %v, want %v", tt.role, got, tt.want)
			}
		})
	}
}
