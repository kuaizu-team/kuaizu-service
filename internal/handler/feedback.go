package handler

import (
	"log"
	"regexp"

	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/labstack/echo/v4"
)

// emailRegex 邮箱格式校验（宽松匹配，与接口文档保持一致）
var emailRegex = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

// CreateFeedback handles POST /feedbacks
func (s *Server) CreateFeedback(ctx echo.Context) error {
	userID := GetUserID(ctx)

	var req api.CreateFeedbackDTO
	if err := ctx.Bind(&req); err != nil {
		return BadRequest(ctx, "请求参数错误")
	}

	// 校验 content：不能为空，不能超过 1000 字符
	if req.Content == "" {
		return Error(ctx, 422, "反馈内容不能为空")
	}
	if len([]rune(req.Content)) > 1000 {
		return Error(ctx, 422, "反馈内容不能超过1000字符")
	}

	// 校验 email 格式（仅在提供时校验）
	if req.Email != nil && *req.Email != "" {
		if !emailRegex.MatchString(*req.Email) {
			return Error(ctx, 422, "邮箱格式不正确")
		}
	}

	feedback := &models.Feedback{
		UserID:  userID,
		Content: req.Content,
		Email:   req.Email,
		Status:  0, // 0 = 待处理
	}

	if err := s.repo.Feedback.Create(ctx.Request().Context(), feedback); err != nil {
		log.Printf("CreateFeedback error: %v", err)
		return InternalError(ctx, "服务器繁忙，请稍后重试")
	}

	return SuccessMessage(ctx, "提交成功")
}
