package service

import (
	"context"
	"time"

	"github.com/trv3wood/kuaizu-server/internal/models"
	"github.com/trv3wood/kuaizu-server/internal/repository"
)

const dailyFreeQuota = 5

// OliveBranchService handles olive branch business logic.
type OliveBranchService struct {
	repo *repository.Repository
}

// NewOliveBranchService creates a new OliveBranchService.
func NewOliveBranchService(repo *repository.Repository) *OliveBranchService {
	return &OliveBranchService{repo: repo}
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
		return nil, ErrInternal("查询橄榄枝状态失败")
	}
	if exists {
		return nil, ErrBadRequest("已有待处理的橄榄枝，请等待对方处理后再发送")
	}

	// Get sender for quota check
	sender, err := s.repo.User.GetByID(ctx, userID)
	if err != nil {
		return nil, ErrInternal("获取用户信息失败")
	}

	// Determine cost type with daily free quota logic
	today := time.Now().Truncate(24 * time.Hour)
	costType := 0

	// Reset free quota if last active date is not today
	if sender.LastActiveDate == nil || sender.LastActiveDate.Truncate(24*time.Hour).Before(today) {
		zero := 0
		sender.FreeBranchUsedToday = &zero
		sender.LastActiveDate = &today
	}

	// Check quota: free first, then paid
	freeBranchUsedToday := 0
	if sender.FreeBranchUsedToday != nil {
		freeBranchUsedToday = *sender.FreeBranchUsedToday
	}
	oliveBranchCount := 0
	if sender.OliveBranchCount != nil {
		oliveBranchCount = *sender.OliveBranchCount
	}

	if freeBranchUsedToday < dailyFreeQuota {
		costType = 1 // Free quota
		freeBranchUsedToday++
		sender.FreeBranchUsedToday = &freeBranchUsedToday
	} else if oliveBranchCount > 0 {
		costType = 2 // Paid quota
		oliveBranchCount--
		sender.OliveBranchCount = &oliveBranchCount
	} else {
		return nil, &ServiceError{Code: ErrorCode(4002), Message: "橄榄枝额度不足，今日免费额度已用完且无付费余额"}
	}

	// Update user quota
	if err := s.repo.User.UpdateQuota(ctx, sender); err != nil {
		return nil, ErrInternal("更新额度失败")
	}

	// Create olive branch record
	ob := &models.OliveBranch{
		SenderID:         userID,
		ReceiverID:       req.ReceiverID,
		RelatedProjectID: req.RelatedProjectID,
		CostType:         costType,
		Status:           0, // 待处理
	}

	if err := s.repo.OliveBranch.Create(ctx, ob); err != nil {
		return nil, ErrInternal("发送橄榄枝失败")
	}

	ob.ProjectName = projectName
	ob.Sender = sender
	ob.Receiver = receiver

	return ob, nil
}

// HandleOliveBranch processes accept/reject of an olive branch.
func (s *OliveBranchService) HandleOliveBranch(ctx context.Context, userID, branchID int, action string) (*models.OliveBranch, error) {
	ob, err := s.repo.OliveBranch.GetByID(ctx, branchID)
	if err != nil {
		return nil, ErrInternal("查询橄榄枝失败")
	}
	if ob == nil {
		return nil, ErrNotFound("橄榄枝不存在")
	}

	if ob.ReceiverID != userID {
		return nil, ErrForbidden("只有接收者可以处理此邀请")
	}

	if ob.Status != 0 {
		return nil, ErrBadRequest("此邀请已被处理")
	}

	var newStatus int
	switch action {
	case "ACCEPT":
		newStatus = 1
	case "REJECT":
		newStatus = 2
	default:
		return nil, ErrBadRequest("操作类型无效，必须为ACCEPT或REJECT")
	}

	if err := s.repo.OliveBranch.UpdateStatus(ctx, branchID, newStatus); err != nil {
		return nil, ErrInternal("处理邀请失败")
	}

	ob.Status = newStatus
	return ob, nil
}
