package handler

import (
	"testing"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

func TestCanAdjustProjectRating(t *testing.T) {
	cases := []struct {
		role int
		want bool
	}{
		{models.AdminRoleSuperAdmin, true},
		{models.AdminRoleSchoolSuperAdmin, true},
		{models.AdminRoleSchoolAdmin, true},
		{models.AdminRoleEventManager, false},
	}
	for _, tc := range cases {
		if got := canAdjustProjectRating(tc.role); got != tc.want {
			t.Fatalf("role %d: got %v, want %v", tc.role, got, tc.want)
		}
	}
}
