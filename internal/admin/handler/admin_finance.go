package handler

import (
	adminvo "github.com/kuaizu-team/kuaizu-service/internal/admin/vo"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/labstack/echo/v4"
)

func canViewAdminDetail(callerRole, callerID int, target *models.AdminUser, callerSchoolID *int) bool {
	if target == nil {
		return false
	}
	if target.ID == callerID {
		return true
	}
	switch callerRole {
	case models.AdminRoleSuperAdmin:
		return true
	case models.AdminRoleSchoolSuperAdmin:
		return (target.Role == models.AdminRoleSchoolAdmin || target.Role == models.AdminRoleEventManager) &&
			schoolIDsMatch(callerSchoolID, target.SchoolID)
	case models.AdminRoleSchoolAdmin:
		return target.Role == models.AdminRoleEventManager &&
			schoolIDsMatch(callerSchoolID, target.SchoolID)
	default:
		return false
	}
}

func canViewAdminPassword(callerRole, callerID int, target *models.AdminUser, callerSchoolID *int) bool {
	if target == nil {
		return false
	}
	if target.ID == callerID {
		return true
	}
	switch callerRole {
	case models.AdminRoleSuperAdmin:
		return target.Role > callerRole
	case models.AdminRoleSchoolSuperAdmin:
		return (target.Role == models.AdminRoleSchoolAdmin || target.Role == models.AdminRoleEventManager) &&
			schoolIDsMatch(callerSchoolID, target.SchoolID)
	default:
		return false
	}
}

func (s *AdminServer) canViewAdminDetailInScope(ctx echo.Context, target *models.AdminUser) bool {
	if target == nil {
		return false
	}
	role := adminRole(ctx)
	if role == models.AdminRoleSuperAdmin {
		return true
	}
	if target.ID == currentAdminID(ctx) {
		return true
	}
	if role == models.AdminRoleSchoolAdmin {
		callerSchoolID := adminSchoolID(ctx)
		return target.Role == models.AdminRoleEventManager &&
			schoolIDsMatch(callerSchoolID, target.SchoolID)
	}
	if role != models.AdminRoleSchoolSuperAdmin ||
		(target.Role != models.AdminRoleSchoolAdmin && target.Role != models.AdminRoleEventManager) {
		return false
	}
	schoolIDs, err := s.adminSchoolIDs(ctx)
	return err == nil && schoolIDInScope(target.SchoolID, schoolIDs)
}

func (s *AdminServer) canViewAdminPasswordInScope(ctx echo.Context, target *models.AdminUser) bool {
	if target == nil {
		return false
	}
	role := adminRole(ctx)
	if target.ID == currentAdminID(ctx) {
		return true
	}
	if role == models.AdminRoleSuperAdmin {
		return target.Role > role
	}
	if role == models.AdminRoleSchoolAdmin {
		return false
	}
	return s.canViewAdminDetailInScope(ctx, target)
}

func attachAdminPassword(vo *adminvo.AdminUserAccountVO, target *models.AdminUser) {
	if vo == nil || target == nil || target.PasswordEncrypted == nil {
		return
	}
	password, err := decryptAdminCredential(*target.PasswordEncrypted)
	if err != nil {
		return
	}
	vo.Password = &password
}

func (s *AdminServer) enrichAdminFinance(ctx echo.Context, vo *adminvo.AdminUserAccountVO, admin *models.AdminUser, includeRemark bool) {
	if vo == nil || admin == nil {
		return
	}
	if !includeRemark {
		vo.FinanceRemark = nil
	}
	if admin.Role == models.AdminRoleSchoolSuperAdmin {
		stats, err := s.repo.Order.AdminRevenueStats(ctx.Request().Context(), admin.ID)
		if err != nil {
			return
		}
		vo.PendingSettlementAmount = stats.PendingSettlementAmount
		vo.PendingRefundOrderCount = stats.PendingRefundOrderCount
		for i := range vo.Schools {
			vo.Schools[i].PendingSettlementAmount = stats.PendingBySchool[vo.Schools[i].SchoolID]
		}
		return
	}
	if admin.Role == models.AdminRoleSchoolAdmin {
		vo.PendingSettlementAmount = 0
		vo.PendingRefundOrderCount = 0
		return
	}
	if admin.SchoolID == nil {
		vo.PendingSettlementAmount = 0
		vo.PendingRefundOrderCount = 0
		return
	}
	stats, err := s.repo.Order.RevenueStats(ctx.Request().Context(), admin.SchoolID)
	if err != nil {
		return
	}
	vo.PendingSettlementAmount = stats.PendingSettlementAmount
	vo.PendingRefundOrderCount = stats.PendingConsumerRefundCount + stats.PendingSchoolAdminRefundCount
}
