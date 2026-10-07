package service

import (
	"context"
	"log"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

// UserService handles user-related business logic.
type UserService struct {
	repo    *repository.Repository
	message *MessageService
}

// NewUserService creates a new UserService.
func NewUserService(repo *repository.Repository, message *MessageService) *UserService {
	return &UserService{repo: repo, message: message}
}

// UserListResult holds a page of users with pagination info.
type UserListResult struct {
	List       []models.User
	Total      int64
	TotalPages int
	Page       int
	Size       int
}

// ListUsers returns a paginated list of users with optional filters.
func (s *UserService) ListUsers(ctx context.Context, params repository.UserListParams) (*UserListResult, error) {
	params.Page, params.Size = normalizePageParams(params.Page, params.Size)

	users, total, err := s.repo.User.ListUsers(ctx, params)
	if err != nil {
		log.Printf("[UserService.ListUsers] repository error: %v", err)
		return nil, ErrInternal("获取用户列表失败")
	}

	totalPages := int((total + int64(params.Size) - 1) / int64(params.Size))
	return &UserListResult{
		List:       users,
		Total:      total,
		TotalPages: totalPages,
		Page:       params.Page,
		Size:       params.Size,
	}, nil
}

// GetUser retrieves a user by ID.
func (s *UserService) GetUser(ctx context.Context, id int) (*models.User, error) {
	user, err := s.repo.User.GetByID(ctx, id)
	if err != nil {
		log.Printf("[UserService.GetUser] repository error: %v", err)
		return nil, ErrInternal("获取用户信息失败")
	}
	if user == nil {
		return nil, ErrNotFound("用户不存在")
	}
	return user, nil
}

// ReviewUserAuth (admin only) updates user's authentication status and notifies user.
func (s *UserService) ReviewUserAuth(ctx context.Context, id, status int) error {
	user, err := s.repo.User.GetByID(ctx, id)
	if err != nil {
		log.Printf("[UserService.ReviewUserAuth] repository error: %v", err)
		return ErrInternal("获取用户信息失败")
	}
	if user == nil {
		return ErrNotFound("用户不存在")
	}

	tx, err := s.repo.DB().BeginTxx(ctx, nil)
	if err != nil {
		return ErrInternal("审核失败")
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "UPDATE `user` SET auth_status=? WHERE id=? AND auth_status <=> ?", status, id, user.AuthStatus)
	if err != nil {
		log.Printf("[UserService.ReviewUserAuth] repository error updating status: %v", err)
		return ErrInternal("审核失败")
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return ErrInternal("审核失败")
	}
	if affected != 1 {
		return ErrBadRequest("认证状态已变化，请刷新后重试")
	}

	resultStr := "认证通过"
	remark := "恭喜！您的身份认证已通过。"
	if status == models.UserAuthStatusFailed {
		resultStr = "认证失败"
		remark = "请修改后重新上传认证资料。"
	}
	data := map[string]string{
		"identity": "在校学生",
		"result":   resultStr,
		"remark":   remark,
	}
	deliveryID, err := s.message.QueueSubscribeTx(ctx, tx, id, models.MsgBizKeyIdentityAuth, data)
	if err != nil {
		return ErrInternal("保存通知失败")
	}
	if err := tx.Commit(); err != nil {
		return ErrInternal("审核失败")
	}
	s.message.DispatchCommittedSubscribe(deliveryID)

	return nil
}
