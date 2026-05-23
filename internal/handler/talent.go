package handler

import (
	"errors"
	"log"

	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/service"
	"github.com/labstack/echo/v4"
)

// ListTalentProfiles handles GET /talent-profiles
func (s *Server) ListTalentProfiles(ctx echo.Context, params api.ListTalentProfilesParams) error {
	page := 1
	size := 10
	if params.Page != nil {
		page = *params.Page
	}
	if params.Size != nil {
		size = *params.Size
	}

	status := models.TalentStatusOnline
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

	var profileVOs []api.TalentProfileVO
	for _, p := range profiles {
		profileVOs = append(profileVOs, *p.ToVO())
	}

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

	userID := GetUserID(ctx)
	source := 0
	if params.Source != nil {
		source = *params.Source
	}

	profile, err := s.svc.TalentProfile.GetTalentProfileWithView(ctx.Request().Context(), id, userID, source)
	if err != nil && params.UserId != nil {
		var svcErr *service.ServiceError
		if errors.As(err, &svcErr) && svcErr.Code == service.ErrCodeNotFound {
			return s.getTalentProfileByUserIDFallback(ctx, *params.UserId)
		}
	}
	if err != nil {
		return mapServiceError(ctx, err)
	}

	return Success(ctx, profile.ToDetailVO())
}

// GetTalentDashboard handles GET /talent-profiles/{id}/dashboard
func (s *Server) GetTalentDashboard(ctx echo.Context, id int) error {
	userID := GetUserID(ctx)
	if id <= 0 {
		return BadRequest(ctx, "无效的人才档案ID")
	}

	result, err := s.svc.TalentProfile.GetTalentDashboard(ctx.Request().Context(), id, userID)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	return Success(ctx, result)
}

// RecordTalentViewDuration handles POST /talent-profiles/{id}/view-duration
func (s *Server) RecordTalentViewDuration(ctx echo.Context, id int) error {
	userID := GetUserID(ctx)
	if id <= 0 {
		return BadRequest(ctx, "无效的人才档案ID")
	}

	var req struct {
		DurationMs int `json:"duration_ms"`
	}
	if err := ctx.Bind(&req); err != nil {
		return BadRequest(ctx, "请求参数错误")
	}

	if err := s.svc.TalentProfile.RecordTalentViewDuration(ctx.Request().Context(), id, userID, req.DurationMs); err != nil {
		return mapServiceError(ctx, err)
	}

	return Success(ctx, nil)
}

// GetTalentViewers handles GET /talent-profiles/{id}/viewers
func (s *Server) GetTalentViewers(ctx echo.Context, id int, params api.GetTalentViewersParams) error {
	userID := GetUserID(ctx)
	if id <= 0 {
		return BadRequest(ctx, "无效的人才档案ID")
	}

	limit := 20
	if params.Limit != nil && *params.Limit > 0 {
		limit = *params.Limit
		if limit > 50 {
			limit = 50
		}
	}

	result, err := s.svc.TalentProfile.GetTalentViewers(ctx.Request().Context(), id, userID, limit)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	return Success(ctx, map[string]any{
		"total": result.Total,
		"list":  result.List,
	})
}

// GetTopTalentViewers handles GET /talent-profiles/{id}/top-viewers
func (s *Server) GetTopTalentViewers(ctx echo.Context, id int, params api.GetTopTalentViewersParams) error {
	userID := GetUserID(ctx)
	if id <= 0 {
		return BadRequest(ctx, "无效的人才档案ID")
	}

	limit := 3
	if params.Limit != nil && *params.Limit > 0 {
		limit = *params.Limit
		if limit > 10 {
			limit = 10
		}
	}

	result, err := s.svc.TalentProfile.GetTopTalentViewers(ctx.Request().Context(), id, userID, limit)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	return Success(ctx, result)
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
