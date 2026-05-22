package handler

import (
	"log"

	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/labstack/echo/v4"
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
		Page:         page,
		Size:         size,
		SchoolID:     params.SchoolId,
		MajorID:      params.MajorId,
		Keyword:      params.Keyword,
		Status:       &status,
		SortBy:       params.SortBy,
		UserSchoolID: params.UserSchoolId,
	}

	// When school_priority sort is requested, pre-fetch the user's school geo info
	// and major class_id so the repository can build the full tier ORDER BY.
	// Lookup failures are non-fatal — the sort gracefully degrades to fewer tiers.
	if params.SortBy != nil && *params.SortBy == "school_priority" {
		reqCtx := ctx.Request().Context()

		if params.UserSchoolId != nil {
			school, err := s.repo.School.GetByID(reqCtx, *params.UserSchoolId)
			if err != nil {
				log.Printf("[ListTalentProfiles] school lookup error (non-fatal): %v", err)
			} else if school != nil {
				listParams.UserSchoolProvince = school.Province
				listParams.UserSchoolCity = school.City
				listParams.UserSchoolDistrict = school.District
			}
		}

		if params.UserMajorId != nil {
			major, err := s.repo.Major.GetByID(reqCtx, *params.UserMajorId)
			if err != nil {
				log.Printf("[ListTalentProfiles] major lookup error (non-fatal): %v", err)
			} else if major != nil {
				classID := major.ClassId
				listParams.UserMajorClassID = &classID
			}
		}
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
	if id <= 0 {
		if params.UserId == nil {
			return BadRequest(ctx, "无效的人才档案ID")
		}
		return s.getTalentProfileByUserIDFallback(ctx, *params.UserId)
	}

	profile, err := s.repo.TalentProfile.GetByID(ctx.Request().Context(), id)
	if err != nil {
		return InternalError(ctx, "获取人才档案失败")
	}

	// 如果人才档案不存在且提供了 userId，回退查找用户基本信息
	if profile == nil && params.UserId != nil {
		return s.getTalentProfileByUserIDFallback(ctx, *params.UserId)
	}

	if profile == nil {
		return NotFound(ctx, "人才档案不存在")
	}

	return Success(ctx, profile.ToDetailVO())
}

func (s *Server) getTalentProfileByUserIDFallback(ctx echo.Context, userID int) error {
	talent, err := s.repo.TalentProfile.GetByUserID(ctx.Request().Context(), userID)
	if err != nil {
		return InternalError(ctx, "获取人才档案失败")
	}
	if talent != nil {
		return Success(ctx, talent.ToDetailVO())
	}

	user, err := s.repo.User.GetByID(ctx.Request().Context(), userID)
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
		Email:      user.Email,
		Phone:      user.Phone,
		WechatID:   user.WechatID,
		Grade:      user.Grade,
		AuthStatus: user.AuthStatus,
	}
	return Success(ctx, talentProfile.ToDetailVO())
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
