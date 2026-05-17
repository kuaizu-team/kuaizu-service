package handler

import (
	"strconv"

	adminvo "github.com/kuaizu-team/kuaizu-service/internal/admin/vo"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
)

// ListFeedbacks handles GET /admin/feedbacks
func (s *AdminServer) ListFeedbacks(ctx echo.Context) error {
	page, _ := strconv.Atoi(ctx.QueryParam("page"))
	size, _ := strconv.Atoi(ctx.QueryParam("size"))

	params := repository.FeedbackListParams{
		Page: page,
		Size: size,
	}

	// 校区管理员自动按学校过滤
	if sid := adminSchoolID(ctx); sid != nil {
		params.SchoolID = sid
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

	// 校区管理员只能查看本校用户的反馈
	if sid := adminSchoolID(ctx); sid != nil {
		if feedback == nil || feedback.UserSchoolID == nil || *feedback.UserSchoolID != *sid {
			return response.Forbidden(ctx, "权限不足")
		}
	}

	return response.Success(ctx, adminvo.NewAdminFeedbackVO(feedback))
}

type replyFeedbackRequest struct {
	AdminReply string `json:"adminReply"`
	// Status is optional. When present (0=待处理, 1=已处理) the DB row is updated accordingly.
	// When omitted the existing status is preserved.
	Status *int `json:"status"`
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

	// status 合法性校验（若传了则必须是 0 或 1）
	if req.Status != nil && *req.Status != 0 && *req.Status != 1 {
		return response.BadRequest(ctx, "status 只能为 0（待处理）或 1（已处理）")
	}

	if err := s.svc.Feedback.ReplyFeedback(ctx.Request().Context(), id, req.AdminReply, req.Status); err != nil {
		return mapServiceError(ctx, err)
	}

	return response.SuccessMessage(ctx, "操作成功")
}
