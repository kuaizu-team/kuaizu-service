package handler

import (
	"testing"

	adminvo "github.com/kuaizu-team/kuaizu-service/internal/admin/vo"
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

func TestSchoolSuperCanOnlyViewEventManagersInScope(t *testing.T) {
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
		{"platform super cannot view another platform super password", models.AdminRoleSuperAdmin, 1, nil, &models.AdminUser{ID: 2, Role: models.AdminRoleSuperAdmin}, false},
		{"platform super views event manager", models.AdminRoleSuperAdmin, 1, nil, &models.AdminUser{ID: 4, Role: models.AdminRoleEventManager, SchoolID: &otherSchoolID}, true},
		{"school super views self", models.AdminRoleSchoolSuperAdmin, 2, &schoolID, &models.AdminUser{ID: 2, Role: models.AdminRoleSchoolSuperAdmin, SchoolID: &schoolID}, true},
		{"school super views same-school admin", models.AdminRoleSchoolSuperAdmin, 2, &schoolID, &models.AdminUser{ID: 3, Role: models.AdminRoleSchoolAdmin, SchoolID: &schoolID}, true},
		{"school super views same-school event manager", models.AdminRoleSchoolSuperAdmin, 2, &schoolID, &models.AdminUser{ID: 4, Role: models.AdminRoleEventManager, SchoolID: &schoolID}, true},
		{"school super cannot view platform super", models.AdminRoleSchoolSuperAdmin, 2, &schoolID, &models.AdminUser{ID: 1, Role: models.AdminRoleSuperAdmin}, false},
		{"school super cannot view other school admin", models.AdminRoleSchoolSuperAdmin, 2, &schoolID, &models.AdminUser{ID: 3, Role: models.AdminRoleSchoolAdmin, SchoolID: &otherSchoolID}, false},
		{"school admin views self", models.AdminRoleSchoolAdmin, 3, &schoolID, &models.AdminUser{ID: 3, Role: models.AdminRoleSchoolAdmin, SchoolID: &schoolID}, true},
		{"school admin cannot view same-school event manager password", models.AdminRoleSchoolAdmin, 3, &schoolID, &models.AdminUser{ID: 4, Role: models.AdminRoleEventManager, SchoolID: &schoolID}, false},
		{"school admin cannot view other-school event manager password", models.AdminRoleSchoolAdmin, 3, &schoolID, &models.AdminUser{ID: 4, Role: models.AdminRoleEventManager, SchoolID: &otherSchoolID}, false},
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

func TestSchoolAdminDetailPermissionMatrix(t *testing.T) {
	schoolID, otherSchoolID := 10, 20
	tests := []struct {
		name   string
		target *models.AdminUser
		want   bool
	}{
		{"self", &models.AdminUser{ID: 3, Role: models.AdminRoleSchoolAdmin, SchoolID: &schoolID}, true},
		{"same-school member", &models.AdminUser{ID: 5, Role: models.AdminRoleSchoolAdmin, SchoolID: &schoolID}, true},
		{"school owner", &models.AdminUser{ID: 2, Role: models.AdminRoleSchoolSuperAdmin, Schools: []models.AdminSchoolRelation{{SchoolID: schoolID, CommissionRate: 70}}}, true},
		{"school owner without access", &models.AdminUser{ID: 9, Role: models.AdminRoleSchoolSuperAdmin, Schools: []models.AdminSchoolRelation{{SchoolID: schoolID}}}, false},
		{"other-school member", &models.AdminUser{ID: 6, Role: models.AdminRoleSchoolAdmin, SchoolID: &otherSchoolID}, false},
		{"same-school event manager", &models.AdminUser{ID: 7, Role: models.AdminRoleEventManager, SchoolID: &schoolID}, true},
		{"other-school event manager", &models.AdminUser{ID: 8, Role: models.AdminRoleEventManager, SchoolID: &otherSchoolID}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := canViewAdminDetail(models.AdminRoleSchoolAdmin, 3, tt.target, &schoolID); got != tt.want {
				t.Fatalf("canViewAdminDetail() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCanEditAdminPermissionMatrix(t *testing.T) {
	schoolID, otherSchoolID := 10, 20
	if canEditAdmin(models.AdminRoleSchoolSuperAdmin, 2, models.AdminRoleSchoolAdmin, 3, &schoolID, &schoolID) {
		t.Fatal("school super admin could edit a school member")
	}
	if canEditAdmin(models.AdminRoleSchoolSuperAdmin, 2, models.AdminRoleEventManager, 4, &schoolID, &otherSchoolID) {
		t.Fatal("school super admin could edit another school's event manager")
	}
	if canEditAdmin(models.AdminRoleSchoolSuperAdmin, 2, models.AdminRoleEventManager, 4, &schoolID, &schoolID) {
		t.Fatal("school super admin could edit a same-school event manager")
	}
	if !canEditAdmin(models.AdminRoleSchoolAdmin, 3, models.AdminRoleSchoolAdmin, 3, &schoolID, &schoolID) {
		t.Fatal("school admin could not edit self")
	}
	if !canEditAdmin(models.AdminRoleEventManager, 4, models.AdminRoleEventManager, 4, &schoolID, &schoolID) {
		t.Fatal("event manager could not edit self")
	}
}

func TestSanitizeSchoolAdminDirectoryVO(t *testing.T) {
	schoolID, otherSchoolID := 10, 20
	password := "secret"
	financeRemark := "private"
	intro := "private intro"
	articleURL := "https://example.com/private"
	target := &models.AdminUser{
		ID: 2, Username: "school-owner", PasswordEncrypted: &password,
		Role: models.AdminRoleSchoolSuperAdmin, FinanceRemark: &financeRemark,
		CommissionRate: 70, Intro: &intro, ArticleURL: &articleURL,
		Schools: []models.AdminSchoolRelation{
			{SchoolID: schoolID, SchoolName: "Current School", CommissionRate: 70},
			{SchoolID: otherSchoolID, SchoolName: "Other School", CommissionRate: 30},
		},
	}
	vo := adminvo.NewAdminUserAccountVO(target)
	vo.Password = &password
	vo.PendingSettlementAmount = 123
	vo.PendingRefundOrderCount = 4

	sanitizeSchoolAdminDirectoryVO(vo, target, &schoolID)

	if vo.Username != "" || vo.Password != nil || vo.FinanceRemark != nil ||
		vo.CommissionRate != 0 || vo.PendingSettlementAmount != 0 ||
		vo.PendingRefundOrderCount != 0 || vo.Intro != nil || vo.ArticleURL != nil {
		t.Fatalf("directory response retained sensitive fields: %#v", vo)
	}
	if vo.SchoolID == nil || *vo.SchoolID != schoolID ||
		vo.SchoolName == nil || *vo.SchoolName != "Current School" {
		t.Fatalf("directory school = (%v, %v), want current school", vo.SchoolID, vo.SchoolName)
	}
	if len(vo.Schools) != 0 {
		t.Fatalf("directory response exposed school relations: %#v", vo.Schools)
	}
}
