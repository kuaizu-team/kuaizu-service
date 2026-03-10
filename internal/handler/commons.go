package handler

import (
	"github.com/labstack/echo/v4"
)

// UploadFile handles POST /commons/uploads
// 根据 form 字段 `type` 区分上传用途：
//   - avatar:     上传用户头像，同时更新 user.avatar_url
//   - background: 上传用户封面图，同时更新 user.cover_image
//   - (其他/空):  仅上传，返回 URL，不更新数据库
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

	default:
		return BadRequest(ctx, "无效的文件类型")
	}
}
