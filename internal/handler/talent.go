package handler

import (
	"github.com/labstack/echo/v4"
	"github.com/trv3wood/kuaizu-server/api"
	"github.com/trv3wood/kuaizu-server/internal/models"
	"github.com/trv3wood/kuaizu-server/internal/repository"
)

// ListTalentProfiles handles GET /talent-profiles
func (s *Server) ListTalentProfiles(ctx echo.Context, params api.ListTalentProfilesParams) error {
	// Set defaults
	page := 1
	size := 10
	if params.Page != nil {
		page = *params.Page
	}
	if params.Size != nil {
		size = *params.Size
	}

	status := models.TalentStatusOnline // 仅展示已发布的
	listParams := repository.TalentProfileListParams{
		Page:     page,
		Size:     size,
		SchoolID: params.SchoolId,
		MajorID:  params.MajorId,
		Keyword:  params.Keyword,
		Status:   &status,
	}

	profiles, total, err := s.repo.TalentProfile.List(ctx.Request().Context(), listParams)
	if err != nil {
		return InternalError(ctx, "获取人才列表失败")
	}

	// Convert to VOs
	var profileVOs []api.TalentProfileVO
	for _, p := range profiles {
		profileVOs = append(profileVOs, *p.ToVO())
	}

	// Build pagination info
	totalPages := int((total + int64(size) - 1) / int64(size))
	response := api.TalentProfilePageResponse{
		List: &profileVOs,
		PageInfo: &api.PageInfo{
			Page:       &page,
			Size:       &size,
			Total:      &total,
			TotalPages: &totalPages,
		},
	}

	return Success(ctx, response)
}

// UpsertTalentProfile handles POST /talent-profiles
func (s *Server) UpsertTalentProfile(ctx echo.Context) error {
	userID := GetUserID(ctx)

	var req api.UpsertTalentProfileDTO
	if err := ctx.Bind(&req); err != nil {
		return InvalidParams(ctx, err)
	}

	updated, err := s.svc.TalentProfile.UpsertTalentProfile(ctx.Request().Context(), userID, req)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	return Success(ctx, updated.ToDetailVO())
}

// GetTalentProfile handles GET /talent-profiles/{id}
func (s *Server) GetTalentProfile(ctx echo.Context, id int, params api.GetTalentProfileParams) error {
	profile, err := s.repo.TalentProfile.GetByID(ctx.Request().Context(), id)
	if err != nil {
		return InternalError(ctx, "获取人才档案失败")
	}

	// 如果人才档案不存在且提供了 userId，回退查找用户基本信息
	if profile == nil && params.UserId != nil {
		user, err := s.repo.User.GetByID(ctx.Request().Context(), *params.UserId)
		if err != nil {
			return InternalError(ctx, "获取用户信息失败")
		}
		if user == nil {
			return NotFound(ctx, "用户不存在")
		}

		talentProfile := models.TalentProfile{
			UserID:     user.ID,
			Nickname:   user.Nickname,
			AvatarUrl:  user.AvatarUrl,
			MajorName:  user.MajorName,
			SchoolName: user.SchoolName,
		}
		// 返回仅包含用户基本信息的响应
		return Success(ctx, talentProfile.ToDetailVO())
	}

	if profile == nil {
		return NotFound(ctx, "人才档案不存在")
	}

	return Success(ctx, profile.ToDetailVO())
}

// GetMyTalentProfile handles GET /users/me/talent-profile
func (s *Server) GetMyTalentProfile(ctx echo.Context) error {
	userID := GetUserID(ctx)

	profile, err := s.repo.TalentProfile.GetByUserID(ctx.Request().Context(), userID)
	if err != nil {
		return InternalError(ctx, "获取人才档案失败")
	}
	if profile == nil {
		return NotFound(ctx, "人才档案不存在")
	}

	return Success(ctx, profile.ToDetailVO())
}

// DeleteMyTalentProfile handles DELETE /talent-profiles/my
func (s *Server) DeleteMyTalentProfile(ctx echo.Context) error {
	userID := GetUserID(ctx)

	if err := s.svc.TalentProfile.SetTalentProfilePrivate(ctx.Request().Context(), userID); err != nil {
		return mapServiceError(ctx, err)
	}

	return Success(ctx, nil)
}
