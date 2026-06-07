package handler

import (
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
)

type invitationConversationStatusRequest struct {
	ConversationStatus string `json:"conversation_status"`
}

type adminInvitationStatusVO struct {
	Status             string     `json:"status"`
	IntentionText      *string    `json:"intention_text"`
	ConversationStatus *string    `json:"conversation_status"`
	UpdatedAt          *time.Time `json:"updated_at"`
}

func (s *AdminServer) GetUserInvitationStatus(ctx echo.Context) error {
	userID, err := parseIDParam(ctx, "id", "user")
	if err != nil {
		return err
	}
	if err := s.ensureAdminCanAccessUser(ctx, userID); err != nil {
		return err
	}

	f, err := s.svc.Invitation.GetStatus(ctx.Request().Context(), userID)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return response.Success(ctx, newAdminInvitationStatusVO(f))
}

func (s *AdminServer) UpdateUserInvitationConversationStatus(ctx echo.Context) error {
	userID, err := parseIDParam(ctx, "id", "user")
	if err != nil {
		return err
	}
	if err := s.ensureAdminCanAccessUser(ctx, userID); err != nil {
		return err
	}

	var req invitationConversationStatusRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}

	f, err := s.svc.Invitation.SetConversationStatus(ctx.Request().Context(), userID, req.ConversationStatus)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return response.Success(ctx, newAdminInvitationStatusVO(f))
}

func newAdminInvitationStatusVO(f *models.InvitationFeedback) adminInvitationStatusVO {
	if f == nil {
		return adminInvitationStatusVO{
			Status: models.InvitationFeedbackStatusPending,
		}
	}
	return adminInvitationStatusVO{
		Status:             f.Status,
		IntentionText:      f.IntentionText,
		ConversationStatus: f.ConversationStatus,
		UpdatedAt:          f.UpdatedAt,
	}
}
