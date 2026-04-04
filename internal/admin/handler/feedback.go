package handler

import (
	"strconv"

	"github.com/labstack/echo/v4"
	adminvo "github.com/kuaizu-team/kuaizu-service/internal/admin/vo"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
)

// ListFeedbacks handles GET /admin/feedbacks
func (s *AdminServer) ListFeedbacks(ctx echo.Context) error {
	page, _ := strconv.Atoi(ctx.QueryParam("page"))
	size, _ := strconv.Atoi(ctx.QueryParam("size"))

	params := repository.FeedbackListParams{
		Page: page,
		Size: size,
	}

	if v := ctx.QueryParam("status"); v != "" {
		status, err := strconv.Atoi(v)
		if err != nil {
			return response.BadRequest(ctx, "invalid status")
		}
		params.Status = &status
	}

	result, err := s.svc.Feedback.ListFeedbacks(ctx.Request().Context(), params)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	list := make([]adminvo.AdminFeedbackVO, len(result.List))
	for i := range result.List {
		list[i] = *adminvo.NewAdminFeedbackVO(&result.List[i])
	}

	return response.Success(ctx, map[string]interface{}{
		"list":  list,
		"total": result.Total,
		"page":  result.Page,
		"size":  result.Size,
	})
}

// GetFeedback handles GET /admin/feedbacks/:id
func (s *AdminServer) GetFeedback(ctx echo.Context) error {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid feedback id")
	}

	feedback, err := s.svc.Feedback.GetFeedback(ctx.Request().Context(), id)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	return response.Success(ctx, adminvo.NewAdminFeedbackVO(feedback))
}

type replyFeedbackRequest struct {
	AdminReply string `json:"adminReply"`
}

// ReplyFeedback handles PATCH /admin/feedbacks/:id
func (s *AdminServer) ReplyFeedback(ctx echo.Context) error {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid feedback id")
	}

	var req replyFeedbackRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}

	if req.AdminReply == "" {
		return response.BadRequest(ctx, "adminReply is required")
	}

	if err := s.svc.Feedback.ReplyFeedback(ctx.Request().Context(), id, req.AdminReply); err != nil {
		return mapServiceError(ctx, err)
	}

	return response.SuccessMessage(ctx, "操作成功")
}
