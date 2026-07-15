package handler

import (
	"strings"

	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/service"
	"github.com/labstack/echo/v4"
)

// GetCurrentUser handles GET /users/me
func (s *Server) GetCurrentUser(ctx echo.Context) error {
	userID := GetUserID(ctx)

	// Update last_active_date and reset daily free quota once per calendar day.
	// This endpoint is called on every mini-program launch, making it the
	// most reliable hook for tracking daily active users without extra round-trips.
	// TouchLastActiveDate only writes to DB when the date has actually changed.
	if err := s.repo.User.TouchLastActiveDate(ctx.Request().Context(), userID); err != nil {
		return InternalError(ctx, "更新额度失败")
	}

	user, err := s.repo.User.GetByID(ctx.Request().Context(), userID)
	if err != nil {
		return InternalError(ctx, "获取用户信息失败")
	}
	if user == nil {
		return NotFound(ctx, "用户不存在")
	}

	return Success(ctx, toExtendedUserVO(user))
}

// GetMyCollaborationHistory handles GET /users/me/collaboration-history.
func (s *Server) GetMyCollaborationHistory(ctx echo.Context) error {
	userID := GetUserID(ctx)

	var list []models.CollaborationScore
	if err := s.repo.DB().SelectContext(ctx.Request().Context(), &list, `
		SELECT cs.id, cs.user_id, cs.project_id, cs.scorer_id, cs.score, cs.created_at,
			p.name AS project_name, u.nickname AS scorer_nickname
		FROM collaboration_score cs
		LEFT JOIN project p ON p.id = cs.project_id
		LEFT JOIN `+"`user`"+` u ON u.id = cs.scorer_id
		WHERE cs.user_id = ?
		ORDER BY cs.created_at DESC, cs.id DESC
	`, userID); err != nil {
		return InternalError(ctx, "get collaboration history failed")
	}
	for i := range list {
		list[i].ScorerNickname = models.DisplayNickname(list[i].ScorerNickname)
	}

	return Success(ctx, list)
}

// SearchUserByPhone handles GET /users/search-by-phone.
func (s *Server) SearchUserByPhone(ctx echo.Context, params api.SearchUserByPhoneParams) error {
	if params.Phone == "" {
		return BadRequest(ctx, "手机号不能为空")
	}
	user, err := s.repo.User.GetByPhone(ctx.Request().Context(), params.Phone)
	if err != nil {
		return InternalError(ctx, "搜索用户失败")
	}
	if user == nil {
		return NotFound(ctx, "用户不存在")
	}
	vo := user.ToVO()
	vo.Phone = nil
	vo.Email = nil
	vo.AuthImgUrl = nil
	vo.CoverImage = nil
	vo.CreatedAt = nil
	vo.Grade = nil
	vo.OliveBranchCount = nil
	vo.FreeBranchUsedToday = nil
	vo.LastActiveDate = nil
	vo.School = nil
	vo.Major = nil
	vo.Skills = nil
	vo.Wechat = nil
	return Success(ctx, vo)
}

// CheckUserByPhone handles GET /user/check-by-phone.
func (s *Server) CheckUserByPhone(ctx echo.Context, params api.CheckUserByPhoneParams) error {
	projectID := 0
	if params.ProjectId != nil {
		projectID = *params.ProjectId
	}

	result, err := s.svc.RegisterInvitation.CheckByPhone(ctx.Request().Context(), params.Phone, projectID)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	data := map[string]interface{}{
		"exists":  result.Exists,
		"invited": result.Invited,
	}
	if result.User != nil {
		vo := result.User.ToVO()
		vo.Phone = nil
		vo.Email = nil
		vo.AuthImgUrl = nil
		vo.CoverImage = nil
		vo.CreatedAt = nil
		vo.Grade = nil
		vo.OliveBranchCount = nil
		vo.FreeBranchUsedToday = nil
		vo.LastActiveDate = nil
		vo.School = nil
		vo.Major = nil
		vo.Skills = nil
		vo.Wechat = nil
		data["user"] = vo
	}
	return Success(ctx, data)
}

// InviteRegister handles POST /invite/register.
func (s *Server) InviteRegister(ctx echo.Context) error {
	var req api.InviteRegisterJSONRequestBody
	if err := ctx.Bind(&req); err != nil {
		return BadRequest(ctx, "请求参数错误")
	}
	role := ""
	if req.Role != nil {
		role = *req.Role
	}
	record, err := s.svc.RegisterInvitation.Invite(ctx.Request().Context(), GetUserID(ctx), service.RegisterInviteInput{
		Phone:     req.Phone,
		ProjectID: req.ProjectId,
		Role:      role,
	})
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return Success(ctx, map[string]interface{}{
		"id":        record.ID,
		"phone":     record.Phone,
		"projectId": record.ProjectID,
		"role":      record.Role,
		"status":    record.Status,
	})
}

// UpdateCurrentUser handles PUT /users/me
func (s *Server) UpdateCurrentUser(ctx echo.Context) error {
	userID := GetUserID(ctx)

	// Bind request body
	var req api.UpdateUserDTO
	if err := ctx.Bind(&req); err != nil {
		return BadRequest(ctx, "请求参数错误")
	}

	// Get existing user
	user, err := s.repo.User.GetByID(ctx.Request().Context(), userID)
	if err != nil {
		return InternalError(ctx, "获取用户信息失败")
	}
	if user == nil {
		return NotFound(ctx, "用户不存在")
	}
	oldEmail := ""
	if user.Email != nil {
		oldEmail = strings.ToLower(strings.TrimSpace(*user.Email))
	}

	// Update fields if provided
	if req.Nickname != nil {
		// 文字内容审核
		if err := s.svc.ContentAudit.CheckText(ctx.Request().Context(), *req.Nickname); err != nil {
			return mapServiceError(ctx, err)
		}
		user.Nickname = req.Nickname
	}
	if req.Phone != nil {
		user.Phone = req.Phone
	}
	if req.Email != nil {
		email := strings.TrimSpace(string(*req.Email))
		user.Email = &email
	}
	if req.SchoolId != nil {
		user.SchoolID = req.SchoolId
	}
	if req.MajorId != nil {
		user.MajorID = req.MajorId
	}
	if req.Grade != nil {
		user.Grade = req.Grade
	}
	if req.Wechat != nil {
		user.WechatID = req.Wechat
	}

	// Save changes
	if err := s.repo.User.Update(ctx.Request().Context(), user); err != nil {
		return InternalError(ctx, "更新用户信息失败")
	}
	newEmail := ""
	if user.Email != nil {
		newEmail = strings.ToLower(strings.TrimSpace(*user.Email))
	}
	if newEmail != "" && newEmail != oldEmail {
		nickname := "同学"
		if user.Nickname != nil && strings.TrimSpace(*user.Nickname) != "" {
			nickname = strings.TrimSpace(*user.Nickname)
		}
		s.svc.WelcomeEmail.Queue(ctx.Request().Context(), userID, newEmail, nickname)
	}

	// Reload user with joined data
	user, err = s.repo.User.GetByID(ctx.Request().Context(), userID)
	if err != nil {
		return InternalError(ctx, "获取用户信息失败")
	}

	return Success(ctx, toExtendedUserVO(user))
}

// SubmitCertification handles POST /users/me/certification
func (s *Server) SubmitCertification(ctx echo.Context) error {
	var req api.SubmitCertificationMultipartRequestBody
	if err := ctx.Bind(&req); err != nil {
		return BadRequest(ctx, "请求参数错误")
	}
	fileHeader, err := ctx.FormFile("studentCertImage")
	if err != nil {
		return BadRequest(ctx, "读取文件失败")
	}
	file, err := fileHeader.Open()
	if err != nil {
		return BadRequest(ctx, "读取文件失败")
	}
	defer file.Close()

	result, err := s.svc.Commons.SubmitCertification(ctx.Request().Context(), GetUserID(ctx), file, fileHeader)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return Success(ctx, result)
}

// GetCertificationStatus handles GET /users/me/certification
func (s *Server) GetCertificationStatus(ctx echo.Context) error {
	certInfo, err := s.repo.User.GetEduCertInfoByID(ctx.Request().Context(), GetUserID(ctx))
	if err != nil {
		return InternalError(ctx, "获取认证状态失败")
	}
	return Success(ctx, api.CertificationStatusVO{
		Status:     (*api.AuthStatus)(&certInfo.Status),
		AuthImgUrl: certInfo.AuthImgUrl,
	})
}
