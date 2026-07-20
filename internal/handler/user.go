package handler

import (
	"strings"

	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
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
// It intentionally returns project-level aggregates only: rater identity,
// individual scores and timestamps are restricted to admin endpoints.
func (s *Server) GetMyCollaborationHistory(ctx echo.Context) error {
	userID := GetUserID(ctx)
	type collaborationProjectSummary struct {
		ID          int64   `db:"id" json:"id"`
		ProjectID   int     `db:"project_id" json:"projectId"`
		ProjectName *string `db:"project_name" json:"projectName,omitempty"`
		Score       float64 `db:"score" json:"score"`
	}

	list := make([]collaborationProjectSummary, 0)
	if err := s.repo.DB().SelectContext(ctx.Request().Context(), &list, `
		SELECT summary.project_id AS id,summary.project_id,p.name AS project_name,summary.score
		FROM (
			SELECT pms.project_id,ROUND(AVG(pms.score),2) AS score,MAX(pms.updated_at) AS sort_at
			FROM project_member_score pms
			WHERE pms.member_id=? AND pms.score IS NOT NULL
			GROUP BY pms.project_id
			UNION ALL
			SELECT cs.project_id,ROUND(AVG(cs.score),2) AS score,MAX(cs.created_at) AS sort_at
			FROM collaboration_score cs
			WHERE cs.user_id=?
				AND NOT EXISTS (
					SELECT 1 FROM project_member_score pms
					WHERE pms.member_id=cs.user_id AND pms.project_id=cs.project_id AND pms.score IS NOT NULL
				)
			GROUP BY cs.project_id
		) summary
		LEFT JOIN project p ON p.id=summary.project_id
		ORDER BY summary.sort_at DESC,summary.project_id DESC
	`, userID, userID); err != nil {
		return InternalError(ctx, "get collaboration history failed")
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
	requestCtx := ctx.Request().Context()

	var req api.UpdateUserDTO
	if err := ctx.Bind(&req); err != nil {
		return BadRequest(ctx, "请求参数错误")
	}
	if req.Nickname != nil {
		if err := s.svc.ContentAudit.CheckText(requestCtx, *req.Nickname); err != nil {
			return mapServiceError(ctx, err)
		}
	}

	tx, err := s.repo.DB().BeginTxx(requestCtx, nil)
	if err != nil {
		return InternalError(ctx, "更新用户信息失败")
	}
	defer tx.Rollback()

	// Lock the user row so concurrent requests can create at most one delivery
	// for the same actual email transition. A later change away and back to the
	// same address remains a separate product event and creates another delivery.
	user, err := s.repo.User.GetByIDForUpdateTx(requestCtx, tx, userID)
	if err != nil {
		return InternalError(ctx, "获取用户信息失败")
	}
	if user == nil {
		return NotFound(ctx, "用户不存在")
	}
	oldEmail := normalizedEmail(user.Email)

	if req.Nickname != nil {
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

	if err := repository.UpdateUserTx(requestCtx, tx, user); err != nil {
		return InternalError(ctx, "更新用户信息失败")
	}

	newEmail := normalizedEmail(user.Email)
	var deliveryID int64
	if newEmail != "" && newEmail != oldEmail {
		deliveryID, err = repository.CreateWelcomeEmailDeliveryTx(requestCtx, tx, userID, newEmail)
		if err != nil {
			return InternalError(ctx, "创建欢迎邮件任务失败")
		}
	}
	if err := tx.Commit(); err != nil {
		return InternalError(ctx, "更新用户信息失败")
	}

	if deliveryID > 0 {
		nickname := "同学"
		if user.Nickname != nil && strings.TrimSpace(*user.Nickname) != "" {
			nickname = strings.TrimSpace(*user.Nickname)
		}
		s.svc.WelcomeEmail.QueueDelivery(deliveryID, userID, newEmail, nickname)
	}

	user, err = s.repo.User.GetByID(requestCtx, userID)
	if err != nil {
		return InternalError(ctx, "获取用户信息失败")
	}
	return Success(ctx, toExtendedUserVO(user))
}

func normalizedEmail(email *string) string {
	if email == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(*email))
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
