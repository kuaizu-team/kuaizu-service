package service

import (
	"context"
	"log"
	"time"

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
	if project.CreatorID != userID {
		return nil, ErrForbidden("只有项目队长可以发送邀请")
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

	// Lock sender row before recalculating and deducting quota to avoid lost updates.
	sender, err := s.repo.User.GetByIDForUpdateTx(ctx, tx, userID)
	if err != nil {
		log.Printf("[OliveBranchService.SendOliveBranch] repository error locking sender: %v", err)
		return nil, ErrInternal("获取用户信息失败")
	}
	if sender == nil {
		return nil, ErrNotFound("用户不存在")
	}

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	quota := models.CalculateOliveBranchQuota(sender, now)
	freeBranchUsedToday := quota.FreeBranchUsedToday
	oliveBranchCount := quota.PaidBalance
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
	sender.LastActiveDate = &today

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
func (s *OliveBranchService) HandleOliveBranch(ctx context.Context, userID, branchID int, action string) (*models.OliveBranch, error) {
	ob, err := s.repo.OliveBranch.GetByID(ctx, branchID)
	if err != nil {
		log.Printf("[OliveBranchService.HandleOliveBranch] repository error getting olive branch: %v", err)
		return nil, ErrInternal("查询橄榄枝失败")
	}
	if ob == nil {
		return nil, ErrNotFound("橄榄枝不存在")
	}

	if ob.ReceiverID != userID {
		return nil, ErrForbidden("只有接收者可以处理此邀请")
	}

	if ob.Status != models.OliveBranchStatusPending {
		return nil, ErrBadRequest("此邀请已被处理")
	}

	var newStatus int
	switch action {
	case "ACCEPT":
		newStatus = models.OliveBranchStatusAccepted
	case "REJECT":
		newStatus = models.OliveBranchStatusRejected
	default:
		return nil, ErrBadRequest("操作类型无效，必须为ACCEPT或REJECT")
	}

	if err := IsValidStatus("olive_branch.status", newStatus); err != nil {
		return nil, err
	}

	if err := s.repo.OliveBranch.UpdateStatus(ctx, branchID, newStatus); err != nil {
		log.Printf("[OliveBranchService.HandleOliveBranch] repository error updating status: %v", err)
		return nil, ErrInternal("处理邀请失败")
	}

	ob.Status = newStatus

	// 向橄榄枝发送方推送「橄榄枝邀请结果通知」（MSG_USER_REPLY）
	// userID 为响应者（接收方），ob.SenderID 为原发送方
	go func(asyncCtx context.Context, senderID, responderID, status int) {
		// 获取响应者昵称（nickname 字段）
		responder, err := s.repo.User.GetByID(asyncCtx, responderID)
		if err != nil || responder == nil {
			log.Printf("[OliveBranchService.HandleOliveBranch] notification: failed to get responder: %v", err)
			return
		}

		responderName := "匿名用户"
		if responder.Nickname != nil && *responder.Nickname != "" {
			responderName = *responder.Nickname
		}

		resultStr := "同意"         // phrase3 ≤ 5 字
		remark := "恭喜！对方已同意您的邀请。" // thing5，13 字 ≤ 20
		if status == models.OliveBranchStatusRejected {
			resultStr = "拒绝"
			remark = "很遗憾，对方拒绝了您的邀请。" // thing5，14 字 ≤ 20
		}

		data := map[string]string{
			"nickname": truncate20(responderName), // thing2 ≤ 20 字
			"result":   resultStr,
			"remark":   remark,
		}

		if err := s.message.SendSubscribeMsgByBizKey(asyncCtx, senderID, models.MsgBizKeyUserReply, data); err != nil {
			log.Printf("[OliveBranchService.HandleOliveBranch] notification error: %v", err)
		}
	}(context.WithoutCancel(ctx), ob.SenderID, userID, newStatus)

	return ob, nil
}
