package handler

import (
	"github.com/labstack/echo/v4"
	"github.com/trv3wood/kuaizu-server/api"
)

// GetCurrentUser handles GET /users/me
func (s *Server) GetCurrentUser(ctx echo.Context) error {
	userID := GetUserID(ctx)

	user, err := s.repo.User.GetByID(ctx.Request().Context(), userID)
	if err != nil {
		return InternalError(ctx, "获取用户信息失败")
	}
	if user == nil {
		return NotFound(ctx, "用户不存在")
	}

	return Success(ctx, user.ToVO())
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

	// Update fields if provided
	if req.Nickname != nil {
		// 文字内容审核
		if err := s.svc.ContentAudit.CheckText(ctx.Request().Context(), *req.Nickname); err != nil {
			return BadRequest(ctx, "昵称包含违规信息，请修改后重试")
		}
		user.Nickname = req.Nickname
	}
	if req.Phone != nil {
		user.Phone = req.Phone
	}
	if req.Email != nil {
		email := string(*req.Email)
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

	// Save changes
	if err := s.repo.User.Update(ctx.Request().Context(), user); err != nil {
		return InternalError(ctx, "更新用户信息失败")
	}

	// Reload user with joined data
	user, err = s.repo.User.GetByID(ctx.Request().Context(), userID)
	if err != nil {
		return InternalError(ctx, "获取用户信息失败")
	}

	return Success(ctx, user.ToVO())
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
		AuthImgUrl: &certInfo.AuthImgUrl,
	})
}
