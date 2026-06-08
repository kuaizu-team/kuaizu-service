package handler

import (
	"net/http"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
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

type pendingInvitationVO struct {
	HasPending bool   `json:"hasPending"`
	Type       string `json:"type"`
}

type nullableResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
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

// GetMyPendingInvitation handles GET /api/v2/users/me/pending-invitation.
func (s *Server) GetMyPendingInvitation(ctx echo.Context) error {
	userID := GetUserID(ctx)

	item, err := s.svc.Invitation.GetPendingInvitation(ctx.Request().Context(), userID)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	var data interface{}
	if item != nil {
		data = pendingInvitationVO{
			HasPending: true,
			Type:       item.InviteType,
		}
	}
	return ctx.JSON(http.StatusOK, nullableResponse{
		Code:    http.StatusOK,
		Message: "操作成功",
		Data:    data,
	})
}

// ClearMyPendingInvitation handles POST /api/v2/users/me/pending-invitation/clear.
func (s *Server) ClearMyPendingInvitation(ctx echo.Context) error {
	userID := GetUserID(ctx)

	if err := s.svc.Invitation.ClearPendingInvitation(ctx.Request().Context(), userID); err != nil {
		return mapServiceError(ctx, err)
	}
	return response.Success(ctx, map[string]bool{"cleared": true})
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
