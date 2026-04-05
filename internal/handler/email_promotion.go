package handler

import (
	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/labstack/echo/v4"
)

// TriggerEmailPromotion 触发邮件推广
// POST /api/email/promotion/trigger
func (s *Server) TriggerEmailPromotion(ctx echo.Context) error {
	userID := GetUserID(ctx)

	var body api.TriggerEmailPromotionJSONRequestBody
	if err := ctx.Bind(&body); err != nil {
		return BadRequest(ctx, "请求参数错误")
	}

	if body.OrderId <= 0 {
		return BadRequest(ctx, "订单ID无效")
	}
	if body.ProjectId <= 0 {
		return BadRequest(ctx, "项目ID无效")
	}

	result, err := s.svc.EmailPromotion.TriggerPromotion(ctx.Request().Context(), userID, body.OrderId, body.ProjectId)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	status := "pending"
	message := "推广任务已创建，正在发送中"

	return Success(ctx, api.TriggerEmailPromotionResponse{
		MaxRecipients: &result.MaxRecipients,
		PromotionId:   &result.Promotion.ID,
		Status:        &status,
		Message:       &message,
	})
}

// GetEmailPromotionStatus 获取推广状态
// GET /api/email/promotion/:id/status
func (s *Server) GetEmailPromotionStatus(ctx echo.Context, id int) error {
	userID := GetUserID(ctx)

	promotion, err := s.svc.EmailPromotion.GetStatus(ctx.Request().Context(), userID, id)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	statusText := map[models.EmailPromotionStatus]string{
		models.EmailPromotionStatusPending:   "待发送",
		models.EmailPromotionStatusSending:   "发送中",
		models.EmailPromotionStatusCompleted: "已完成",
		models.EmailPromotionStatusFailed:    "发送失败",
	}

	statusTextValue := statusText[promotion.Status]
	statusValue := api.EmailPromotionStatus(promotion.Status)

	return Success(ctx, api.EmailPromotionVO{
		Id:            &promotion.ID,
		ProjectId:     &promotion.ProjectID,
		ProjectName:   promotion.ProjectName,
		MaxRecipients: &promotion.MaxRecipients,
		TotalSent:     &promotion.TotalSent,
		Status:        &statusValue,
		StatusText:    &statusTextValue,
		ErrorMessage:  promotion.ErrorMessage,
		StartedAt:     promotion.StartedAt,
		CompletedAt:   promotion.CompletedAt,
		CreatedAt:     &promotion.CreatedAt,
	})
}

// ListMyEmailPromotions 获取我的推广记录
// GET /api/email/promotions/my
func (s *Server) ListMyEmailPromotions(ctx echo.Context) error {
	userID := GetUserID(ctx)

	page := 1
	size := 10

	promotions, total, err := s.svc.EmailPromotion.ListByCreator(ctx.Request().Context(), userID, page, size)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	statusText := map[models.EmailPromotionStatus]string{
		models.EmailPromotionStatusPending:   "待发送",
		models.EmailPromotionStatusSending:   "发送中",
		models.EmailPromotionStatusCompleted: "已完成",
		models.EmailPromotionStatusFailed:    "发送失败",
	}

	list := make([]api.EmailPromotionVO, len(promotions))
	for i, p := range promotions {
		statusTextValue := statusText[p.Status]
		statusValue := api.EmailPromotionStatus(p.Status)

		list[i] = api.EmailPromotionVO{
			Id:            &p.ID,
			ProjectId:     &p.ProjectID,
			ProjectName:   p.ProjectName,
			MaxRecipients: &p.MaxRecipients,
			TotalSent:     &p.TotalSent,
			Status:        &statusValue,
			StatusText:    &statusTextValue,
			CreatedAt:     &p.CreatedAt,
		}
	}

	return Success(ctx, map[string]interface{}{
		"list":  list,
		"total": total,
	})
}
