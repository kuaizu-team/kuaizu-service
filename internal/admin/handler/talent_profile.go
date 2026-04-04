package handler

import (
	"fmt"

	"github.com/labstack/echo/v4"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
)

type reviewTalentProfileRequest struct {
	Status int `json:"status"`
}

// ReviewTalentProfile handles PATCH /admin/talent-profiles/:id
func (s *AdminServer) ReviewTalentProfile(ctx echo.Context) error {
	id, err := parseIDParam(ctx, "id", "talent profile")
	if err != nil {
		return err
	}

	var req reviewTalentProfileRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}

	if req.Status != models.TalentStatusOnline && req.Status != models.TalentStatusPrivate {
		return response.BadRequest(ctx, fmt.Sprintf("invalid status %d, must be %d (approve) or %d (reject)", req.Status, models.TalentStatusOnline, models.TalentStatusPrivate))
	}

	if err := s.svc.TalentProfile.ReviewTalentProfile(ctx.Request().Context(), id, req.Status); err != nil {
		return mapServiceError(ctx, err)
	}

	return response.SuccessMessage(ctx, "操作成功")
}
