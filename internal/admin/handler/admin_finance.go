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
	switch callerRole {
	case models.AdminRoleSuperAdmin:
		return true
	case models.AdminRoleSchoolSuperAdmin:
		if target.ID == callerID {
			return true
		}
		return target.Role == models.AdminRoleEventManager || (target.Role == models.AdminRoleSchoolAdmin && schoolIDsMatch(callerSchoolID, target.SchoolID))
	default:
		return false
	}
}

func (s *AdminServer) enrichAdminFinance(ctx echo.Context, vo *adminvo.AdminUserAccountVO, admin *models.AdminUser, includeRemark bool) {
	if vo == nil || admin == nil {
		return
	}
	if !includeRemark {
		vo.FinanceRemark = nil
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
