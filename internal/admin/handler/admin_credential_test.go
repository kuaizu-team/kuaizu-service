package handler

import (
	"testing"

	adminvo "github.com/kuaizu-team/kuaizu-service/internal/admin/vo"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

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
	financeRemark := "private"
	intro := "private intro"
	articleURL := "https://example.com/private"
	target := &models.AdminUser{
		ID: 2, Username: "school-owner",
		Role: models.AdminRoleSchoolSuperAdmin, FinanceRemark: &financeRemark,
		CommissionRate: 70, Intro: &intro, ArticleURL: &articleURL,
		Schools: []models.AdminSchoolRelation{
			{SchoolID: schoolID, SchoolName: "Current School", CommissionRate: 70},
			{SchoolID: otherSchoolID, SchoolName: "Other School", CommissionRate: 30},
		},
	}
	vo := adminvo.NewAdminUserAccountVO(target)
	vo.PendingSettlementAmount = 123
	vo.PendingRefundOrderCount = 4

	sanitizeSchoolAdminDirectoryVO(vo, target, &schoolID)

	if vo.Username != "" || vo.FinanceRemark != nil ||
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

func TestCanViewAdminFinanceOverview(t *testing.T) {
	schoolSuper := &models.AdminUser{ID: 2, Role: models.AdminRoleSchoolSuperAdmin}
	schoolAdmin := &models.AdminUser{ID: 3, Role: models.AdminRoleSchoolAdmin}
	eventManager := &models.AdminUser{ID: 4, Role: models.AdminRoleEventManager}

	if !canViewAdminFinanceOverview(models.AdminRoleSuperAdmin, 1, schoolSuper) {
		t.Fatal("platform super admin could not view a school super admin finance overview")
	}
	if !canViewAdminFinanceOverview(models.AdminRoleSchoolSuperAdmin, 2, schoolSuper) {
		t.Fatal("school super admin could not view own finance overview")
	}
	if canViewAdminFinanceOverview(models.AdminRoleSchoolAdmin, 3, schoolSuper) {
		t.Fatal("school admin could view a school super admin finance overview")
	}
	if canViewAdminFinanceOverview(models.AdminRoleSuperAdmin, 1, schoolAdmin) {
		t.Fatal("school admin target exposed a finance overview")
	}
	if canViewAdminFinanceOverview(models.AdminRoleEventManager, 4, eventManager) {
		t.Fatal("event manager target exposed a finance overview")
	}
}

func TestClearAdminFinanceOverview(t *testing.T) {
	remark := "private"
	vo := &adminvo.AdminUserAccountVO{
		PendingSettlementAmount: 123,
		PendingRefundOrderCount: 4,
		FinanceRemark:           &remark,
		Schools: []adminvo.AdminSchoolVO{{
			SchoolID: 10, PendingSettlementAmount: 88,
		}},
	}

	clearAdminFinanceOverview(vo)

	if vo.PendingSettlementAmount != 0 || vo.PendingRefundOrderCount != 0 ||
		vo.FinanceRemark != nil || vo.Schools[0].PendingSettlementAmount != 0 {
		t.Fatalf("finance overview was not cleared: %#v", vo)
	}
}
