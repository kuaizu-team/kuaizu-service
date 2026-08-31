package handler

import (
	"github.com/labstack/echo/v4"
)

// UploadFile handles POST /commons/uploads
// 根据 form 字段 `type` 区分上传用途：
//   - avatar:     上传用户头像，同时更新 user.avatar_url
//   - background: 上传用户封面图，同时更新 user.cover_image
//   - project_image/talent_work/milestone_evidence: 上传到独立业务目录
func (s *Server) UploadFile(ctx echo.Context) error {
	file, header, err := ctx.Request().FormFile("file")
	if err != nil {
		return BadRequest(ctx, "缺少文件字段 'file'")
	}
	defer file.Close()

	uploadType := ctx.FormValue("type")

	switch uploadType {
	case "avatar":
		userID := GetUserID(ctx)
		result, err := s.svc.Commons.UploadAvatar(ctx.Request().Context(), userID, file, header)
		if err != nil {
			return mapServiceError(ctx, err)
		}
		return Success(ctx, map[string]string{"url": result.URL})

	case "background":
		userID := GetUserID(ctx)
		result, err := s.svc.Commons.UploadCoverImage(ctx.Request().Context(), userID, file, header)
		if err != nil {
			return mapServiceError(ctx, err)
		}
		return Success(ctx, map[string]string{"url": result.URL})

	case "project_image", "talent_work", "milestone_evidence":
		result, err := s.svc.Commons.UploadBusinessImage(file, header, uploadType)
		if err != nil {
			return mapServiceError(ctx, err)
		}
		userID := GetUserID(ctx)
		if err := s.repo.Media.RegisterUpload(ctx.Request().Context(), result.Key, userID, uploadType); err != nil {
			_ = s.svc.Commons.DeleteFile(result.Key)
			return InternalError(ctx, "记录上传图片失败")
		}
		return Success(ctx, map[string]string{"url": result.URL, "key": result.Key})

	default:
		return BadRequest(ctx, "无效的文件类型")
	}
}
