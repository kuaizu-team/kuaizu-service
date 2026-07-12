package handler

import (
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
)

// ListWebsiteTeam only exposes fields intentionally used by the public website.
func (s *Server) ListWebsiteTeam(ctx echo.Context) error {
	status := 1
	admins, _, err := s.repo.AdminUser.List(ctx.Request().Context(), repository.AdminUserListParams{Page: 1, Size: 100, Status: &status})
	if err != nil {
		return response.InternalError(ctx, "获取团队信息失败")
	}
	result := make([]map[string]interface{}, 0, len(admins))
	for _, a := range admins {
		if a.Role != models.AdminRoleSchoolSuperAdmin && a.Role != models.AdminRoleSchoolAdmin {
			continue
		}
		result = append(result, map[string]interface{}{"id": a.ID, "nickname": a.Nickname, "role": a.Role, "schoolName": a.SchoolName, "joinDate": a.JoinDate, "intro": a.Intro, "articleUrl": a.ArticleURL})
	}
	return response.Success(ctx, result)
}
