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
