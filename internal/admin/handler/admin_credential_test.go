package handler

import (
	"testing"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

func TestAdminCredentialRoundTrip(t *testing.T) {
	t.Setenv("ADMIN_CREDENTIAL_KEY", "test-only-credential-key")
	encrypted, err := encryptAdminCredential("event-secret-123")
	if err != nil {
		t.Fatalf("encryptAdminCredential returned error: %v", err)
	}
	if encrypted == "event-secret-123" {
		t.Fatal("credential was stored as plaintext")
	}
	plainText, err := decryptAdminCredential(encrypted)
	if err != nil {
		t.Fatalf("decryptAdminCredential returned error: %v", err)
	}
	if plainText != "event-secret-123" {
		t.Fatalf("decrypted value = %q", plainText)
	}
}

func TestSchoolSuperCannotViewOtherSchoolEventManager(t *testing.T) {
	callerSchoolID, otherSchoolID := 10, 20
	target := &models.AdminUser{ID: 9, Role: models.AdminRoleEventManager, SchoolID: &otherSchoolID}
	if canViewAdminDetail(models.AdminRoleSchoolSuperAdmin, 2, target, &callerSchoolID) {
		t.Fatal("school super admin could view another school's event manager")
	}
	target.SchoolID = &callerSchoolID
	if !canViewAdminDetail(models.AdminRoleSchoolSuperAdmin, 2, target, &callerSchoolID) {
		t.Fatal("school super admin could not view same-school event manager")
	}
}

func TestCanViewAdminPasswordPermissionMatrix(t *testing.T) {
	schoolID, otherSchoolID := 10, 20
	tests := []struct {
		name           string
		callerRole     int
		callerID       int
		callerSchoolID *int
		target         *models.AdminUser
		want           bool
	}{
		{"platform super views another platform super", models.AdminRoleSuperAdmin, 1, nil, &models.AdminUser{ID: 2, Role: models.AdminRoleSuperAdmin}, true},
		{"platform super views event manager", models.AdminRoleSuperAdmin, 1, nil, &models.AdminUser{ID: 4, Role: models.AdminRoleEventManager, SchoolID: &otherSchoolID}, true},
		{"school super views self", models.AdminRoleSchoolSuperAdmin, 2, &schoolID, &models.AdminUser{ID: 2, Role: models.AdminRoleSchoolSuperAdmin, SchoolID: &schoolID}, true},
		{"school super views same-school admin", models.AdminRoleSchoolSuperAdmin, 2, &schoolID, &models.AdminUser{ID: 3, Role: models.AdminRoleSchoolAdmin, SchoolID: &schoolID}, true},
		{"school super views same-school event manager", models.AdminRoleSchoolSuperAdmin, 2, &schoolID, &models.AdminUser{ID: 4, Role: models.AdminRoleEventManager, SchoolID: &schoolID}, true},
		{"school super cannot view platform super", models.AdminRoleSchoolSuperAdmin, 2, &schoolID, &models.AdminUser{ID: 1, Role: models.AdminRoleSuperAdmin}, false},
		{"school super cannot view other school admin", models.AdminRoleSchoolSuperAdmin, 2, &schoolID, &models.AdminUser{ID: 3, Role: models.AdminRoleSchoolAdmin, SchoolID: &otherSchoolID}, false},
		{"school admin views self", models.AdminRoleSchoolAdmin, 3, &schoolID, &models.AdminUser{ID: 3, Role: models.AdminRoleSchoolAdmin, SchoolID: &schoolID}, true},
		{"school admin cannot view another admin", models.AdminRoleSchoolAdmin, 3, &schoolID, &models.AdminUser{ID: 4, Role: models.AdminRoleEventManager, SchoolID: &schoolID}, false},
		{"event manager views self", models.AdminRoleEventManager, 4, &schoolID, &models.AdminUser{ID: 4, Role: models.AdminRoleEventManager, SchoolID: &schoolID}, true},
		{"event manager cannot view another admin", models.AdminRoleEventManager, 4, &schoolID, &models.AdminUser{ID: 3, Role: models.AdminRoleSchoolAdmin, SchoolID: &schoolID}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := canViewAdminPassword(tt.callerRole, tt.callerID, tt.target, tt.callerSchoolID); got != tt.want {
				t.Fatalf("canViewAdminPassword() = %v, want %v", got, tt.want)
			}
		})
	}
}
