package handler

import (
	"strings"

	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/labstack/echo/v4"
)

// SearchRoleTags handles GET /role-tags.
func (s *Server) SearchRoleTags(ctx echo.Context, params api.SearchRoleTagsParams) error {
	keyword := ""
	if params.Keyword != nil {
		keyword = strings.TrimSpace(*params.Keyword)
	}
	roleCode := ""
	if params.RoleCode != nil {
		roleCode = strings.TrimSpace(*params.RoleCode)
	}
	if len([]rune(keyword)) > 32 || len([]rune(roleCode)) > 32 {
		return BadRequest(ctx, "标签关键词或角色编码过长")
	}
	limit := 10
	if params.Limit != nil {
		limit = *params.Limit
	}
	if limit < 1 {
		limit = 10
	} else if limit > 20 {
		limit = 20
	}

	tags, err := s.repo.RoleTag.Search(ctx.Request().Context(), keyword, roleCode, limit)
	if err != nil {
		return InternalError(ctx, "搜索角色标签失败")
	}
	return Success(ctx, tags)
}
