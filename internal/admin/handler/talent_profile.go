package handler

import (
	"fmt"
	"strings"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
)

type reviewTalentProfileRequest struct {
	Status       int     `json:"status"`
	RejectReason *string `json:"rejectReason"`
	Reason       *string `json:"reason"`
}

type talentProfileReasonRequest struct {
	RejectReason *string `json:"rejectReason"`
	Reason       *string `json:"reason"`
}

func normalizeTalentProfileReason(rejectReason *string, reason *string) (string, error) {
	value := ""
	if rejectReason != nil {
		value = *rejectReason
	} else if reason != nil {
		value = *reason
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("reason is required")
	}
	if len([]rune(value)) > 25 {
		return "", fmt.Errorf("reason must be within 25 characters")
	}
	return value, nil
}

// checkTalentProfileSchool returns 403 if the talent profile does not belong to the admin's school.
// GetByID already JOINs the user table and exposes SchoolID, so no extra query is needed.
func (s *AdminServer) checkTalentProfileSchool(ctx echo.Context, id int) error {
	if adminRole(ctx) == models.AdminRoleSchoolSuperAdmin {
		tp, err := s.repo.TalentProfile.GetByID(ctx.Request().Context(), id)
		if err != nil {
			return response.InternalError(ctx, "查询名片失败")
		}
		if tp == nil {
			return response.NotFound(ctx, "名片不存在")
		}
		allowed, err := s.canAccessSchool(ctx, tp.SchoolID)
		if err != nil || !allowed {
			return response.Forbidden(ctx, "权限不足")
		}
		return nil
	}
	sid := adminSchoolID(ctx)
	if sid == nil {
		return nil // super admin — no restriction
	}
	tp, err := s.repo.TalentProfile.GetByID(ctx.Request().Context(), id)
	if err != nil {
		return response.InternalError(ctx, "查询名片失败")
	}
	if tp == nil || tp.SchoolID == nil || *tp.SchoolID != *sid {
		return response.Forbidden(ctx, "权限不足")
	}
	return nil
}

// TakedownTalentProfile handles PATCH /admin/talent-profiles/:id/takedown
func (s *AdminServer) TakedownTalentProfile(ctx echo.Context) error {
	id, err := parseIDParam(ctx, "id", "talent profile")
	if err != nil {
		return err
	}

	// 校区管理员只能操作本校用户的名片
	if err := s.checkTalentProfileSchool(ctx, id); err != nil {
		return err
	}

	var req talentProfileReasonRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	reason, err := normalizeTalentProfileReason(req.RejectReason, req.Reason)
	if err != nil {
		return response.BadRequest(ctx, err.Error())
	}

	if err := s.svc.TalentProfile.TakedownTalentProfile(ctx.Request().Context(), id, reason); err != nil {
		return mapServiceError(ctx, err)
	}

	return response.SuccessMessage(ctx, "操作成功")
}

// ReviewTalentProfile handles PATCH /admin/talent-profiles/:id
func (s *AdminServer) ReviewTalentProfile(ctx echo.Context) error {
	id, err := parseIDParam(ctx, "id", "talent profile")
	if err != nil {
		return err
	}

	// 校区管理员只能操作本校用户的名片
	if err := s.checkTalentProfileSchool(ctx, id); err != nil {
		return err
	}

	var req reviewTalentProfileRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}

	if req.Status != models.TalentStatusOnline && req.Status != models.TalentStatusPrivate {
		return response.BadRequest(ctx, fmt.Sprintf("invalid status %d, must be %d (approve) or %d (reject)", req.Status, models.TalentStatusOnline, models.TalentStatusPrivate))
	}

	var reason *string
	if req.Status == models.TalentStatusPrivate {
		value, err := normalizeTalentProfileReason(req.RejectReason, req.Reason)
		if err != nil {
			return response.BadRequest(ctx, err.Error())
		}
		reason = &value
	}

	if err := s.svc.TalentProfile.ReviewTalentProfile(ctx.Request().Context(), id, req.Status, reason); err != nil {
		return mapServiceError(ctx, err)
	}

	return response.SuccessMessage(ctx, "操作成功")
}
