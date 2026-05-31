package handler

import (
	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/service"
	"github.com/labstack/echo/v4"
)

func (s *Server) PostSmsSend(ctx echo.Context) error {
	userID := GetUserID(ctx)

	var req api.PostSmsSendJSONRequestBody
	if err := ctx.Bind(&req); err != nil {
		return BadRequest(ctx, "请求参数错误")
	}

	notice, err := s.svc.SmsNotice.Send(ctx.Request().Context(), userID, service.SendSmsNoticeInput{
		OrderID:             req.OrderId,
		ReceiverUserID:      req.ReceiverUserId,
		OliveBranchRecordID: req.OliveBranchRecordId,
		ProjectID:           req.ProjectId,
	})
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return Success(ctx, smsNoticeToVO(notice))
}

func (s *Server) GetSmsSendRecordId(ctx echo.Context, recordId int) error {
	userID := GetUserID(ctx)
	notice, err := s.svc.SmsNotice.GetByID(ctx.Request().Context(), userID, recordId)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return Success(ctx, smsNoticeToVO(notice))
}

func (s *Server) GetSmsSendByOliveBranchOliveBranchRecordId(ctx echo.Context, oliveBranchRecordId int) error {
	userID := GetUserID(ctx)
	notice, err := s.svc.SmsNotice.GetByOliveBranchRecordID(ctx.Request().Context(), userID, oliveBranchRecordId)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return Success(ctx, smsNoticeToVO(notice))
}

func smsNoticeToVO(notice *models.SmsNotice) api.SmsNoticeVO {
	status := int(notice.Status)
	message := smsNoticeStatusMessage(notice.Status)
	return api.SmsNoticeVO{
		Id:                  &notice.ID,
		RecordId:            &notice.ID,
		OrderId:             &notice.OrderID,
		ReceiverUserId:      &notice.ReceiverID,
		OliveBranchRecordId: &notice.OliveBranchRecordID,
		ProjectId:           notice.ProjectID,
		Status:              &status,
		Message:             &message,
		ErrorMessage:        notice.ErrorMessage,
		CreatedAt:           &notice.CreatedAt,
		UpdatedAt:           &notice.UpdatedAt,
	}
}

func smsNoticeStatusMessage(status models.SmsNoticeStatus) string {
	switch status {
	case models.SmsNoticeStatusPending:
		return "待发送"
	case models.SmsNoticeStatusSending:
		return "发送中"
	case models.SmsNoticeStatusCompleted:
		return "已完成"
	case models.SmsNoticeStatusFailed:
		return "发送失败"
	default:
		return "未知状态"
	}
}
