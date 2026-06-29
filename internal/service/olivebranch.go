package service

import (
	"context"
	"log"
	"strings"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

// OliveBranchService handles olive branch business logic.
type OliveBranchService struct {
	repo    *repository.Repository
	message *MessageService
}

// NewOliveBranchService creates a new OliveBranchService.
func NewOliveBranchService(repo *repository.Repository, message *MessageService) *OliveBranchService {
	return &OliveBranchService{repo: repo, message: message}
}

// SendRequest holds the input for sending an olive branch.
type SendRequest struct {
	ReceiverID       int
	RelatedProjectID int
}

// SendOliveBranch validates and creates an olive branch record with quota management.
func (s *OliveBranchService) SendOliveBranch(ctx context.Context, userID int, req SendRequest) (*models.OliveBranch, error) {
	// Validate receiver exists
	receiver, err := s.repo.User.GetByID(ctx, req.ReceiverID)
	if err != nil {
		log.Printf("[OliveBranchService.SendOliveBranch] repository error getting receiver: %v", err)
		return nil, ErrInternal("查询用户失败")
	}
	if receiver == nil {
		return nil, ErrNotFound("接收用户不存在")
	}

	// Cannot send to self
	if req.ReceiverID == userID {
		return nil, ErrBadRequest("不能向自己发送橄榄枝")
	}

	// Validate project
	var projectName *string
	project, err := s.repo.Project.GetByID(ctx, req.RelatedProjectID)
	if err != nil {
		log.Printf("[OliveBranchService.SendOliveBranch] repository error getting project: %v", err)
		return nil, ErrInternal("查询项目失败")
	}
	if project == nil {
		return nil, ErrNotFound("关联项目不存在")
	}
	members, err := s.repo.Project.ListMembers(ctx, req.RelatedProjectID)
	if err != nil {
		log.Printf("[OliveBranchService.SendOliveBranch] repository error listing project members: %v", err)
		return nil, ErrInternal("检查项目成员权限失败")
	}
	operatorRole, operatorRoleName := currentUserProjectRole(project, userID, members)
	if operatorRole == nil {
		return nil, ErrForbidden("只有项目团队成员可以发送邀请")
	}
	projectName = &project.Name

	// Check for duplicate pending olive branch
	exists, err := s.repo.OliveBranch.ExistsPending(ctx, userID, req.ReceiverID, req.RelatedProjectID)
	if err != nil {
		log.Printf("[OliveBranchService.SendOliveBranch] repository error checking duplicate: %v", err)
		return nil, ErrInternal("查询橄榄枝状态失败")
	}
	if exists {
		return nil, ErrBadRequest("已有待处理的橄榄枝，请等待对方处理后再发送")
	}

	tx, err := s.repo.DB().BeginTxx(ctx, nil)
	if err != nil {
		log.Printf("[OliveBranchService.SendOliveBranch] failed to begin transaction: %v", err)
		return nil, ErrInternal("发送橄榄枝失败")
	}
	defer tx.Rollback()

	if err := s.repo.User.ResetDailyFreeBranchQuotaIfNeededTx(ctx, tx, userID); err != nil {
		log.Printf("[OliveBranchService.SendOliveBranch] repository error resetting daily quota: %v", err)
		return nil, ErrInternal("更新额度失败")
	}

	// Lock sender row before recalculating and deducting quota to avoid lost updates.
	sender, err := s.repo.User.GetByIDForUpdateTx(ctx, tx, userID)
	if err != nil {
		log.Printf("[OliveBranchService.SendOliveBranch] repository error locking sender: %v", err)
		return nil, ErrInternal("获取用户信息失败")
	}
	if sender == nil {
		return nil, ErrNotFound("用户不存在")
	}

	freeBranchUsedToday := 0
	if sender.FreeBranchUsedToday != nil {
		freeBranchUsedToday = *sender.FreeBranchUsedToday
	}
	oliveBranchCount := 0
	if sender.OliveBranchCount != nil {
		oliveBranchCount = *sender.OliveBranchCount
	}
	costType := 0

	if freeBranchUsedToday < models.OliveBranchDailyFreeQuota {
		costType = models.OliveBranchCostFree // Free quota
		freeBranchUsedToday++
		sender.FreeBranchUsedToday = &freeBranchUsedToday
	} else if oliveBranchCount > 0 {
		costType = models.OliveBranchCostPaid // Paid quota
		oliveBranchCount--
		sender.OliveBranchCount = &oliveBranchCount
	} else {
		return nil, &ServiceError{Code: ErrorCode(4002), Message: "橄榄枝额度不足，今日免费额度已用完且无付费余额"}
	}
	sender.OliveBranchCount = &oliveBranchCount

	// Update user quota
	if err := s.repo.User.UpdateQuotaTx(ctx, tx, sender); err != nil {
		log.Printf("[OliveBranchService.SendOliveBranch] repository error updating quota: %v", err)
		return nil, ErrInternal("更新额度失败")
	}

	// Create olive branch record
	ob := &models.OliveBranch{
		SenderID:         userID,
		ReceiverID:       req.ReceiverID,
		RelatedProjectID: req.RelatedProjectID,
		CostType:         costType,
		Status:           models.OliveBranchStatusPending,
		OperatorRole:     operatorRole,
	}

	if err := s.repo.OliveBranch.CreateTx(ctx, tx, ob); err != nil {
		log.Printf("[OliveBranchService.SendOliveBranch] repository error creating olive branch: %v", err)
		return nil, ErrInternal("发送橄榄枝失败")
	}

	if err := tx.Commit(); err != nil {
		log.Printf("[OliveBranchService.SendOliveBranch] failed to commit transaction: %v", err)
		return nil, ErrInternal("发送橄榄枝失败")
	}

	ob.ProjectName = projectName
	ob.OperatorRoleName = operatorRoleName
	ob.Sender = sender
	ob.Receiver = receiver

	// 向接收方推送「收到橄榄枝通知」（MSG_INVITE_JOIN）
	// sender、project 均已在本函数内查询，直接捕获，无额外 DB 开销
	go func(asyncCtx context.Context) {
		inviterName := "匿名用户"
		if sender.Nickname != nil && *sender.Nickname != "" {
			inviterName = *sender.Nickname
		}

		data := map[string]string{
			"project_name": truncate20(project.Name), // thing1 ≤ 20 字
			"inviter":      truncate20(inviterName),  // thing2 ≤ 20 字
			"remark":       "您收到一条橄榄枝邀请",             // thing4，10 字 ≤ 20
		}

		if err := s.message.SendSubscribeMsgByBizKey(asyncCtx, req.ReceiverID, models.MsgBizKeyInviteJoin, data); err != nil {
			log.Printf("[OliveBranchService.SendOliveBranch] notification error: %v", err)
		}
	}(context.WithoutCancel(ctx))

	return ob, nil
}

// ResendOliveBranch reactivates an existing olive branch record instead of creating a duplicate.
func (s *OliveBranchService) ResendOliveBranch(ctx context.Context, userID, branchID int) (*models.OliveBranch, error) {
	ob, err := s.repo.OliveBranch.GetByID(ctx, branchID)
	if err != nil {
		log.Printf("[OliveBranchService.ResendOliveBranch] repository error getting olive branch: %v", err)
		return nil, ErrInternal("查询橄榄枝失败")
	}
	if ob == nil {
		return nil, ErrNotFound("橄榄枝不存在")
	}
	if ob.Status == models.OliveBranchStatusPending || ob.Status == models.OliveBranchStatusDiscussing {
		return nil, ErrBadRequest("当前状态不需要再次发送")
	}

	project, err := s.repo.Project.GetByID(ctx, ob.RelatedProjectID)
	if err != nil {
		log.Printf("[OliveBranchService.ResendOliveBranch] repository error getting project: %v", err)
		return nil, ErrInternal("查询项目失败")
	}
	if project == nil {
		return nil, ErrNotFound("关联项目不存在")
	}
	members, err := s.repo.Project.ListMembers(ctx, ob.RelatedProjectID)
	if err != nil {
		log.Printf("[OliveBranchService.ResendOliveBranch] repository error listing members: %v", err)
		return nil, ErrInternal("检查项目成员失败")
	}
	currentRole, operatorRoleName := currentUserProjectRole(project, userID, members)
	if currentRole == nil {
		return nil, ErrForbidden("只有项目团队成员可以再次发送橄榄枝")
	}
	for _, member := range members {
		if member.UserID == ob.ReceiverID {
			return nil, ErrBadRequest("该用户仍在团队中，无需再次发送")
		}
	}

	tx, err := s.repo.DB().BeginTxx(ctx, nil)
	if err != nil {
		return nil, ErrInternal("再次发送橄榄枝失败")
	}
	defer tx.Rollback()
	if err := s.repo.User.ResetDailyFreeBranchQuotaIfNeededTx(ctx, tx, userID); err != nil {
		return nil, ErrInternal("更新额度失败")
	}
	sender, err := s.repo.User.GetByIDForUpdateTx(ctx, tx, userID)
	if err != nil || sender == nil {
		return nil, ErrInternal("获取用户额度失败")
	}
	freeUsed, paidBalance := 0, 0
	if sender.FreeBranchUsedToday != nil {
		freeUsed = *sender.FreeBranchUsedToday
	}
	if sender.OliveBranchCount != nil {
		paidBalance = *sender.OliveBranchCount
	}
	if freeUsed < models.OliveBranchDailyFreeQuota {
		freeUsed++
		sender.FreeBranchUsedToday = &freeUsed
		ob.CostType = models.OliveBranchCostFree
	} else if paidBalance > 0 {
		paidBalance--
		sender.OliveBranchCount = &paidBalance
		ob.CostType = models.OliveBranchCostPaid
	} else {
		return nil, &ServiceError{Code: ErrorCode(4002), Message: "橄榄枝额度不足，今日免费额度已用完且无付费余额"}
	}
	if err := s.repo.User.UpdateQuotaTx(ctx, tx, sender); err != nil {
		return nil, ErrInternal("更新额度失败")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE olive_branch_record SET status=?, cost_type=?, is_read=FALSE, updated_at=CURRENT_TIMESTAMP WHERE id=?`, models.OliveBranchStatusPending, ob.CostType, branchID); err != nil {
		return nil, ErrInternal("再次发送橄榄枝失败")
	}
	if err := tx.Commit(); err != nil {
		return nil, ErrInternal("再次发送橄榄枝失败")
	}

	ob.Status = models.OliveBranchStatusPending
	ob.IsRead = false
	ob.OperatorRole = currentRole
	ob.OperatorRoleName = operatorRoleName
	ob.ProjectName = &project.Name

	go func(asyncCtx context.Context) {
		sender, err := s.repo.User.GetByID(asyncCtx, userID)
		if err != nil || sender == nil {
			log.Printf("[OliveBranchService.ResendOliveBranch] notification: failed to get sender: %v", err)
			return
		}
		inviterName := "匿名用户"
		if sender.Nickname != nil && *sender.Nickname != "" {
			inviterName = *sender.Nickname
		}
		data := map[string]string{
			"project_name": truncate20(project.Name),
			"inviter":      truncate20(inviterName),
			"remark":       "您收到一条新的橄榄枝邀请",
		}
		if err := s.message.SendSubscribeMsgByBizKey(asyncCtx, ob.ReceiverID, models.MsgBizKeyInviteJoin, data); err != nil {
			log.Printf("[OliveBranchService.ResendOliveBranch] notification error: %v", err)
		}
	}(context.WithoutCancel(ctx))

	return ob, nil
}

// HandleOliveBranch processes accept/reject of an olive branch.
func (s *OliveBranchService) HandleOliveBranch(ctx context.Context, userID, branchID int, action string, role string) (*models.OliveBranch, error) {
	ob, err := s.repo.OliveBranch.GetByID(ctx, branchID)
	if err != nil {
		log.Printf("[OliveBranchService.HandleOliveBranch] repository error getting olive branch: %v", err)
		return nil, ErrInternal("\u67e5\u8be2\u6a44\u6984\u679d\u5931\u8d25")
	}
	if ob == nil {
		return nil, ErrNotFound("\u6a44\u6984\u679d\u4e0d\u5b58\u5728")
	}
	action = strings.ToUpper(strings.TrimSpace(action))
	role = strings.TrimSpace(role)
	switch action {
	case "ACCEPT", "DISCUSS":
		if ob.ReceiverID != userID {
			return nil, ErrForbidden("\u65e0\u6743\u5904\u7406\u8be5\u6a44\u6984\u679d")
		}
		if ob.Status != models.OliveBranchStatusPending {
			return nil, ErrBadRequest("\u5f53\u524d\u72b6\u6001\u4e0d\u53ef\u64cd\u4f5c")
		}
		if err := s.repo.OliveBranch.UpdateStatus(ctx, branchID, models.OliveBranchStatusDiscussing); err != nil {
			log.Printf("[OliveBranchService.HandleOliveBranch] repository error updating status: %v", err)
			return nil, ErrInternal("\u66f4\u65b0\u72b6\u6001\u5931\u8d25")
		}
		ob.Status = models.OliveBranchStatusDiscussing
		s.notifyOliveBranchResult(ctx, ob.SenderID, userID, ob.Status)
		return ob, nil
	case "REJECT":
		wasDiscussing := ob.Status == models.OliveBranchStatusDiscussing
		rejectedByReceiver := ob.ReceiverID == userID
		if ob.Status == models.OliveBranchStatusPending {
			if ob.ReceiverID != userID {
				return nil, ErrForbidden("\u65e0\u6743\u5904\u7406\u8be5\u6a44\u6984\u679d")
			}
		} else if ob.Status == models.OliveBranchStatusDiscussing {
			if !rejectedByReceiver {
				if err := s.ensureCanOperateDiscussingBranch(ctx, ob, userID, ""); err != nil {
					return nil, err
				}
			}
		} else {
			return nil, ErrBadRequest("\u5f53\u524d\u72b6\u6001\u4e0d\u53ef\u64cd\u4f5c")
		}
		if !wasDiscussing {
			if err := s.repo.OliveBranch.UpdateStatus(ctx, branchID, models.OliveBranchStatusRejected); err != nil {
				log.Printf("[OliveBranchService.HandleOliveBranch] repository error updating status: %v", err)
				return nil, ErrInternal("\u66f4\u65b0\u72b6\u6001\u5931\u8d25")
			}
		} else {
			tx, err := s.repo.DB().BeginTxx(ctx, nil)
			if err != nil {
				return nil, ErrInternal("\u5f00\u542f\u4e8b\u52a1\u5931\u8d25")
			}
			defer tx.Rollback()
			if _, err := tx.ExecContext(ctx, `UPDATE olive_branch_record SET status=?, rejected_at=CURRENT_TIMESTAMP, updated_at=CURRENT_TIMESTAMP WHERE id=?`, models.OliveBranchStatusRejected, branchID); err != nil {
				return nil, ErrInternal("\u66f4\u65b0\u72b6\u6001\u5931\u8d25")
			}
			if !rejectedByReceiver {
				if err := repository.CreateOliveStatusNotificationTx(ctx, tx, ob.ReceiverID, branchID, models.StatusNotificationOliveRejected); err != nil {
					return nil, ErrInternal("\u521b\u5efa\u72b6\u6001\u901a\u77e5\u5931\u8d25")
				}
			}
			if err := tx.Commit(); err != nil {
				return nil, ErrInternal("\u63d0\u4ea4\u4e8b\u52a1\u5931\u8d25")
			}
		}
		ob.Status = models.OliveBranchStatusRejected
		s.notifyOliveBranchResult(ctx, ob.SenderID, userID, ob.Status)
		return ob, nil
	case "ADMIT":
		if ob.Status != models.OliveBranchStatusDiscussing {
			return nil, ErrBadRequest("\u8bf7\u5148\u8fdb\u5165\u4e92\u76f8\u4e86\u89e3\u540e\u518d\u51c6\u8bb8\u5165\u961f")
		}
		if role == "" {
			return nil, ErrBadRequest("\u8bf7\u9009\u62e9\u56e2\u961f\u89d2\u8272")
		}
		if err := s.ensureCanOperateDiscussingBranch(ctx, ob, userID, role); err != nil {
			return nil, err
		}
		tx, err := s.repo.DB().BeginTxx(ctx, nil)
		if err != nil {
			return nil, ErrInternal("\u5f00\u542f\u4e8b\u52a1\u5931\u8d25")
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, `INSERT INTO project_members(project_id,user_id,role) VALUES(?,?,?) ON DUPLICATE KEY UPDATE role=VALUES(role), updated_at=CURRENT_TIMESTAMP`, ob.RelatedProjectID, ob.ReceiverID, role); err != nil {
			log.Printf("[OliveBranchService.HandleOliveBranch] add member failed: %v", err)
			return nil, ErrInternal("\u52a0\u5165\u56e2\u961f\u5931\u8d25")
		}
		if _, err := tx.ExecContext(ctx, `UPDATE olive_branch_record SET status=?, assigned_role=?, admitted_at=CURRENT_TIMESTAMP, updated_at=CURRENT_TIMESTAMP WHERE id=?`, models.OliveBranchStatusAccepted, role, branchID); err != nil {
			log.Printf("[OliveBranchService.HandleOliveBranch] update status failed: %v", err)
			return nil, ErrInternal("\u66f4\u65b0\u72b6\u6001\u5931\u8d25")
		}
		if err := repository.CreateOliveStatusNotificationTx(ctx, tx, ob.ReceiverID, branchID, models.StatusNotificationOliveAccepted); err != nil {
			return nil, ErrInternal("\u521b\u5efa\u72b6\u6001\u901a\u77e5\u5931\u8d25")
		}
		if err := tx.Commit(); err != nil {
			return nil, ErrInternal("\u63d0\u4ea4\u4e8b\u52a1\u5931\u8d25")
		}
		ob.Status = models.OliveBranchStatusAccepted
		ob.AssignedRole = &role
		s.notifyOliveBranchResult(ctx, ob.ReceiverID, userID, ob.Status)
		return ob, nil
	default:
		return nil, ErrBadRequest("\u4e0d\u652f\u6301\u7684\u64cd\u4f5c")
	}
}

func (s *OliveBranchService) ensureCanOperateDiscussingBranch(ctx context.Context, ob *models.OliveBranch, userID int, assignedRole string) error {
	project, err := s.repo.Project.GetByID(ctx, ob.RelatedProjectID)
	if err != nil {
		log.Printf("[OliveBranchService.ensureCanOperateDiscussingBranch] repository error getting project: %v", err)
		return ErrInternal("\u67e5\u8be2\u9879\u76ee\u5931\u8d25")
	}
	if project == nil {
		return ErrNotFound("\u9879\u76ee\u4e0d\u5b58\u5728")
	}
	members, err := s.repo.Project.ListMembers(ctx, ob.RelatedProjectID)
	if err != nil {
		log.Printf("[OliveBranchService.ensureCanOperateDiscussingBranch] repository error listing members: %v", err)
		return ErrInternal("\u67e5\u8be2\u56e2\u961f\u6210\u5458\u5931\u8d25")
	}
	currentRole, _ := currentUserProjectRole(project, userID, members)
	if currentRole == nil {
		return ErrForbidden("\u65e0\u6743\u7ba1\u7406\u8be5\u9879\u76ee")
	}
	if assignedRole != "" {
		if !canAssignProjectRole(currentRole, assignedRole) {
			return ErrForbidden("\u65e0\u6743\u5206\u914d\u8be5\u56e2\u961f\u89d2\u8272")
		}
		exists, err := s.repo.Project.RoleExists(ctx, assignedRole)
		if err != nil {
			return ErrInternal("\u67e5\u8be2\u89d2\u8272\u5931\u8d25")
		}
		if !exists {
			return ErrBadRequest("\u89d2\u8272\u4e0d\u5b58\u5728")
		}
	}
	return nil
}

func (s *OliveBranchService) notifyOliveBranchResult(ctx context.Context, targetUserID, responderID, status int) {
	go func(asyncCtx context.Context) {
		responder, err := s.repo.User.GetByID(asyncCtx, responderID)
		if err != nil || responder == nil {
			log.Printf("[OliveBranchService.HandleOliveBranch] notification: failed to get responder: %v", err)
			return
		}
		responderName := "\u5bf9\u65b9"
		if responder.Nickname != nil && *responder.Nickname != "" {
			responderName = *responder.Nickname
		}
		resultStr := "\u5df2\u8fdb\u5165\u4e92\u76f8\u4e86\u89e3"
		remark := "\u8bf7\u524d\u5f80\u9879\u76ee\u6216\u4eba\u624d\u8be6\u60c5\u7ee7\u7eed\u6c9f\u901a"
		if status == models.OliveBranchStatusRejected {
			resultStr = "\u4e0d\u5408\u9002"
			remark = "\u5bf9\u65b9\u6682\u672a\u7ee7\u7eed\u63a8\u8fdb\u672c\u6b21\u9080\u7ea6"
		} else if status == models.OliveBranchStatusAccepted {
			resultStr = "\u5df2\u5165\u961f"
			remark = "\u5df2\u51c6\u8bb8\u52a0\u5165\u56e2\u961f"
		}
		data := map[string]string{"nickname": truncate20(responderName), "result": resultStr, "remark": remark}
		if err := s.message.SendSubscribeMsgByBizKey(asyncCtx, targetUserID, models.MsgBizKeyUserReply, data); err != nil {
			log.Printf("[OliveBranchService.HandleOliveBranch] notification error: %v", err)
		}
	}(context.WithoutCancel(ctx))
}
