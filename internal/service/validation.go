package service

import (
	"fmt"

	"github.com/trv3wood/kuaizu-server/internal/models"
)

// IsValidStatus checks if a given status value is within the valid range for the specified field.
func IsValidStatus(field string, status int) error {
	switch field {
	case "olive_branch.status":
		// 状态:0-待处理,1-已接受,2-已拒绝,3-已忽略
		if status < models.OliveBranchStatusPending || status > models.OliveBranchStatusIgnored {
			return ErrBadRequest(fmt.Sprintf("无效的橄榄枝状态: %d", status))
		}
	case "order.status":
		// 支付状态:0-待支付,1-已支付,2-已取消,3-已退款
		if status < models.OrderStatusPending || status > models.OrderStatusRefunded {
			return ErrBadRequest(fmt.Sprintf("无效的订单状态: %d", status))
		}
	case "product.type":
		// 类型:1-虚拟币,2-服务权益
		if status < models.ProductTypeCurrency || status > models.ProductTypeBenefit {
			return ErrBadRequest(fmt.Sprintf("无效的商品类型: %d", status))
		}
	case "project.direction":
		// 项目方向:1-落地,2-比赛,3-学习
		if status < models.ProjectDirectionLaunch || status > models.ProjectDirectionLearning {
			return ErrBadRequest(fmt.Sprintf("无效的项目方向: %d", status))
		}
	case "project.status":
		// 审核状态:0-待审核,1-已通过,2-已驳回,3-已关闭
		if status < models.ProjectStatusPending || status > models.ProjectStatusClosed {
			return ErrBadRequest(fmt.Sprintf("无效的项目状态: %d", status))
		}
	case "project.promotion_status":
		// 推广状态:0-无,1-推广中,2-已结束
		if status < models.ProjectPromotionNone || status > models.ProjectPromotionFinished {
			return ErrBadRequest(fmt.Sprintf("无效的推广状态: %d", status))
		}
	case "project.is_cross_school":
		// 是否跨校: 1-可以,0-不可以
		if status < models.ProjectCrossSchoolNo || status > models.ProjectCrossSchoolYes {
			return ErrBadRequest(fmt.Sprintf("无效的跨校设置: %d", status))
		}
	case "project.education_requirement":
		// 学历要求1-大专2-本科3-研究生
		if status < models.EducationJuniorCollege || status > models.EducationPostgraduate {
			return ErrBadRequest(fmt.Sprintf("无效的学历要求: %d", status))
		}
	case "application.status":
		// 状态:0-待审核,1-已通过,2-已拒绝
		if status < models.ApplicationStatusPending || status > models.ApplicationStatusRejected {
			return ErrBadRequest(fmt.Sprintf("无效的申请状态: %d", status))
		}
	case "talent_profile.status":
		// 状态:0-隐私/下架,1-上架,2-审核中
		if status < models.TalentStatusPrivate || status > models.TalentStatusReviewing {
			return ErrBadRequest(fmt.Sprintf("无效的人才档案状态: %d", status))
		}
	case "user.auth_status":
		// 认证状态:0-未认证,1-已认证,2-认证失败
		if status < models.UserAuthStatusNone || status > models.UserAuthStatusFailed {
			return ErrBadRequest(fmt.Sprintf("无效的认证状态: %d", status))
		}
	default:
		return nil
	}
	return nil
}
