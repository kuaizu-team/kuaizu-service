package handler

import (
	"fmt"
	"strconv"

	adminvo "github.com/kuaizu-team/kuaizu-service/internal/admin/vo"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
)

// ListTalentProfiles handles GET /admin/talent-profiles
func (s *AdminServer) ListTalentProfiles(ctx echo.Context) error {
	page, _ := strconv.Atoi(ctx.QueryParam("page"))
	size, _ := strconv.Atoi(ctx.QueryParam("size"))
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 10
	}
	if size > 50 {
		size = 50
	}

	params := repository.TalentProfileAdminListParams{
		Page: page,
		Size: size,
	}

	if v := ctx.QueryParam("status"); v != "" {
		status, err := strconv.Atoi(v)
		if err != nil || (status != models.TalentStatusPrivate && status != models.TalentStatusOnline && status != models.TalentStatusReviewing) {
			return response.BadRequest(ctx, "invalid status, must be 0, 1 or 2")
		}
		params.Status = &status
	}

	if v := ctx.QueryParam("keyword"); v != "" {
		params.Keyword = &v
	}

	profiles, total, err := s.svc.TalentProfile.AdminListTalentProfiles(ctx.Request().Context(), params)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	list := make([]adminvo.AdminTalentProfileVO, len(profiles))
	for i := range profiles {
		list[i] = *adminvo.NewAdminTalentProfileVO(&profiles[i])
	}

	return response.Success(ctx, map[string]interface{}{
		"list":  list,
		"total": total,
		"page":  page,
		"size":  size,
	})
}

// GetTalentProfile handles GET /admin/talent-profiles/:id
func (s *AdminServer) GetTalentProfile(ctx echo.Context) error {
	id, err := parseIDParam(ctx, "id", "talent profile")
	if err != nil {
		return err
	}

	profile, err := s.svc.TalentProfile.AdminGetTalentProfile(ctx.Request().Context(), id)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	return response.Success(ctx, adminvo.NewAdminTalentProfileDetailVO(profile))
}

// TakedownTalentProfile handles PUT /admin/talent-profiles/:id/takedown
func (s *AdminServer) TakedownTalentProfile(ctx echo.Context) error {
	id, err := parseIDParam(ctx, "id", "talent profile")
	if err != nil {
		return err
	}

	if err := s.svc.TalentProfile.TakedownTalentProfile(ctx.Request().Context(), id); err != nil {
		return mapServiceError(ctx, err)
	}

	return response.SuccessMessage(ctx, "已下架")
}

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
