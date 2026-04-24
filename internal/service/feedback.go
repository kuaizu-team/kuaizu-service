package service

import (
	"context"
	"log"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

// FeedbackService handles feedback-related business logic.
type FeedbackService struct {
	repo    *repository.Repository
	message *MessageService
}

// NewFeedbackService creates a new FeedbackService.
func NewFeedbackService(repo *repository.Repository, message *MessageService) *FeedbackService {
	return &FeedbackService{repo: repo, message: message}
}

// FeedbackListResult holds a page of feedbacks with pagination info.
type FeedbackListResult struct {
	List       []models.Feedback
	Total      int64
	TotalPages int
	Page       int
	Size       int
}

// ListFeedbacks returns a paginated list of feedbacks with optional filters.
func (s *FeedbackService) ListFeedbacks(ctx context.Context, params repository.FeedbackListParams) (*FeedbackListResult, error) {
	params.Page, params.Size = normalizePageParams(params.Page, params.Size)

	feedbacks, total, err := s.repo.Feedback.List(ctx, params)
	if err != nil {
		log.Printf("[FeedbackService.ListFeedbacks] repository error: %v", err)
		return nil, ErrInternal("获取反馈列表失败")
	}

	totalPages := int((total + int64(params.Size) - 1) / int64(params.Size))
	return &FeedbackListResult{
		List:       feedbacks,
		Total:      total,
		TotalPages: totalPages,
		Page:       params.Page,
		Size:       params.Size,
	}, nil
}

// GetFeedback retrieves a feedback by ID.
func (s *FeedbackService) GetFeedback(ctx context.Context, id int) (*models.Feedback, error) {
	fb, err := s.repo.Feedback.GetByID(ctx, id)
	if err != nil {
		log.Printf("[FeedbackService.GetFeedback] repository error: %v", err)
		return nil, ErrInternal("获取反馈详情失败")
	}
	if fb == nil {
		return nil, ErrNotFound("反馈不存在")
	}
	return fb, nil
}

// ReplyFeedback (admin only) replies to a feedback and notifies user.
func (s *FeedbackService) ReplyFeedback(ctx context.Context, id int, reply string) error {
	fb, err := s.repo.Feedback.GetByID(ctx, id)
	if err != nil {
		log.Printf("[FeedbackService.ReplyFeedback] repository error: %v", err)
		return ErrInternal("获取反馈信息失败")
	}
	if fb == nil {
		return ErrNotFound("反馈不存在")
	}

	if err := s.repo.Feedback.Reply(ctx, id, reply); err != nil {
		log.Printf("[FeedbackService.ReplyFeedback] repository error: %v", err)
		return ErrInternal("回复反馈失败")
	}

	return nil
}
