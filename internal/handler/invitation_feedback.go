package handler

import (
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/labstack/echo/v4"
)

type invitationFeedbackRequest struct {
	Status        string  `json:"status"`
	IntentionText *string `json:"intention_text"`
}

type invitationFeedbackVO struct {
	Status             string     `json:"status"`
	IntentionText      *string    `json:"intention_text"`
	ConversationStatus *string    `json:"conversation_status"`
	UpdatedAt          *time.Time `json:"updated_at"`
}

// SubmitInvitationFeedback handles POST /api/v2/invitation/feedback.
func (s *Server) SubmitInvitationFeedback(ctx echo.Context) error {
	userID := GetUserID(ctx)

	var req invitationFeedbackRequest
	if err := ctx.Bind(&req); err != nil {
		return BadRequest(ctx, "invalid request body")
	}

	f, err := s.svc.Invitation.SubmitFeedback(ctx.Request().Context(), userID, req.Status, req.IntentionText)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return Success(ctx, newInvitationFeedbackVO(f))
}

func newInvitationFeedbackVO(f *models.InvitationFeedback) invitationFeedbackVO {
	if f == nil {
		return invitationFeedbackVO{
			Status: models.InvitationFeedbackStatusPending,
		}
	}
	return invitationFeedbackVO{
		Status:             f.Status,
		IntentionText:      f.IntentionText,
		ConversationStatus: f.ConversationStatus,
		UpdatedAt:          f.UpdatedAt,
	}
}
