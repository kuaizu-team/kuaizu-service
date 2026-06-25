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

// HandleOliveBranch processes accept/reject of an olive branch.
func (s *OliveBranchService) HandleOliveBranch(ctx context.Context, userID, branchID int, action string, role string) (*models.OliveBranch, error) {
	ob, err := s.repo.OliveBranch.GetByID(ctx, branchID)
	if err != nil {
		log.Printf("[OliveBranchService.HandleOliveBranch] repository error getting olive branch: %v", err)
		return nil, ErrInternal("???????")
	}
	if ob == nil {
		return nil, ErrNotFound("??????")
	}
	action = strings.ToUpper(strings.TrimSpace(action))
	role = strings.TrimSpace(role)
	switch action {
	case "ACCEPT", "DISCUSS":
		if ob.ReceiverID != userID {
			return nil, ErrForbidden("????????????")
		}
		if ob.Status != models.OliveBranchStatusPending {
			return nil, ErrBadRequest("???????")
		}
		if err := s.repo.OliveBranch.UpdateStatus(ctx, branchID, models.OliveBranchStatusDiscussing); err != nil {
			log.Printf("[OliveBranchService.HandleOliveBranch] repository error updating status: %v", err)
			return nil, ErrInternal("??????")
		}
		ob.Status = models.OliveBranchStatusDiscussing
		s.notifyOliveBranchResult(ctx, ob.SenderID, userID, ob.Status)
		return ob, nil
	case "REJECT":
		if ob.Status == models.OliveBranchStatusPending {
			if ob.ReceiverID != userID {
				return nil, ErrForbidden("????????????")
			}
		} else if ob.Status == models.OliveBranchStatusDiscussing {
			if err := s.ensureCanOperateDiscussingBranch(ctx, ob, userID, ""); err != nil {
				return nil, err
			}
		} else {
			return nil, ErrBadRequest("???????")
		}
		if err := s.repo.OliveBranch.UpdateStatus(ctx, branchID, models.OliveBranchStatusRejected); err != nil {
			log.Printf("[OliveBranchService.HandleOliveBranch] repository error updating status: %v", err)
			return nil, ErrInternal("??????")
		}
		ob.Status = models.OliveBranchStatusRejected
		s.notifyOliveBranchResult(ctx, ob.SenderID, userID, ob.Status)
		return ob, nil
	case "ADMIT":
		if ob.Status != models.OliveBranchStatusDiscussing {
			return nil, ErrBadRequest("?????????????????")
		}
		if role == "" {
			return nil, ErrBadRequest("???????")
		}
		if err := s.ensureCanOperateDiscussingBranch(ctx, ob, userID, role); err != nil {
			return nil, err
		}
		tx, err := s.repo.DB().BeginTxx(ctx, nil)
		if err != nil {
			return nil, ErrInternal("??????")
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, `INSERT INTO project_members(project_id,user_id,role) VALUES(?,?,?) ON DUPLICATE KEY UPDATE role=VALUES(role), updated_at=CURRENT_TIMESTAMP`, ob.RelatedProjectID, ob.ReceiverID, role); err != nil {
			log.Printf("[OliveBranchService.HandleOliveBranch] add member failed: %v", err)
			return nil, ErrInternal("????????")
		}
		if _, err := tx.ExecContext(ctx, `UPDATE olive_branch_record SET status=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, models.OliveBranchStatusAccepted, branchID); err != nil {
			log.Printf("[OliveBranchService.HandleOliveBranch] update status failed: %v", err)
			return nil, ErrInternal("??????")
		}
		if err := tx.Commit(); err != nil {
			return nil, ErrInternal("??????")
		}
		ob.Status = models.OliveBranchStatusAccepted
		ob.AssignedRole = &role
		s.notifyOliveBranchResult(ctx, ob.ReceiverID, userID, ob.Status)
		return ob, nil
	default:
		return nil, ErrBadRequest("??????")
	}
}

func (s *OliveBranchService) ensureCanOperateDiscussingBranch(ctx context.Context, ob *models.OliveBranch, userID int, assignedRole string) error {
	project, err := s.repo.Project.GetByID(ctx, ob.RelatedProjectID)
	if err != nil {
		log.Printf("[OliveBranchService.ensureCanOperateDiscussingBranch] repository error getting project: %v", err)
		return ErrInternal("????????")
	}
	if project == nil {
		return ErrNotFound("?????")
	}
	members, err := s.repo.Project.ListMembers(ctx, ob.RelatedProjectID)
	if err != nil {
		log.Printf("[OliveBranchService.ensureCanOperateDiscussingBranch] repository error listing members: %v", err)
		return ErrInternal("??????")
	}
	currentRole, _ := currentUserProjectRole(project, userID, members)
	if currentRole == nil {
		return ErrForbidden("???????????")
	}
	if assignedRole != "" {
		if !canAssignProjectRole(currentRole, assignedRole) {
			return ErrForbidden("?????????????")
		}
		exists, err := s.repo.Project.RoleExists(ctx, assignedRole)
		if err != nil {
			return ErrInternal("????????")
		}
		if !exists {
			return ErrBadRequest("???????")
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
		responderName := "????"
		if responder.Nickname != nil && *responder.Nickname != "" {
			responderName = *responder.Nickname
		}
		resultStr := "??????"
		remark := "??????????"
		if status == models.OliveBranchStatusRejected {
			resultStr = "??"
			remark = "??????????"
		} else if status == models.OliveBranchStatusAccepted {
			resultStr = "???"
			remark = "????????"
		}
		data := map[string]string{"nickname": truncate20(responderName), "result": resultStr, "remark": remark}
		if err := s.message.SendSubscribeMsgByBizKey(asyncCtx, targetUserID, models.MsgBizKeyUserReply, data); err != nil {
			log.Printf("[OliveBranchService.HandleOliveBranch] notification error: %v", err)
		}
	}(context.WithoutCancel(ctx))
}
