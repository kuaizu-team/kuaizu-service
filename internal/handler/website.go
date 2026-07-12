package handler

import (
	"database/sql"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
	"time"
)

type websiteContentItem struct {
	ID           int    `db:"id" json:"id"`
	Title        string `db:"title" json:"title"`
	Content      string `db:"content" json:"content"`
	ArticleURL   string `db:"url" json:"articleUrl"`
	DisplayOrder int    `db:"display_order" json:"displayOrder"`
}

type websiteEventItem struct {
	ID                   int        `db:"id" json:"id"`
	Name                 string     `db:"name" json:"name"`
	IsRanking            bool       `db:"is_ranking" json:"isRanking"`
	RegistrationDeadline *time.Time `db:"registration_deadline" json:"registrationDeadline"`
	ArticleURL           *string    `db:"article_url" json:"articleUrl"`
}

func (s *Server) listWebsiteContent(ctx echo.Context, category string) error {
	items := make([]websiteContentItem, 0)
	err := s.repo.DB().SelectContext(ctx.Request().Context(), &items, `
		SELECT id, title, content, url, display_order
		FROM information_content
		WHERE category = ? AND is_published = 1
		ORDER BY display_order DESC, created_at DESC, id DESC`, category)
	if err != nil {
		return response.InternalError(ctx, "获取官网内容失败")
	}
	return response.Success(ctx, items)
}

func (s *Server) ListWebsitePodcast(ctx echo.Context) error {
	return s.listWebsiteContent(ctx, "kuaizu_talking")
}
func (s *Server) ListWebsiteProjects(ctx echo.Context) error {
	return s.listWebsiteContent(ctx, "campus_project")
}
func (s *Server) ListWebsiteTalent(ctx echo.Context) error {
	return s.listWebsiteContent(ctx, "talent")
}

func (s *Server) ListWebsiteEvents(ctx echo.Context) error {
	items := make([]websiteEventItem, 0)
	err := s.repo.DB().SelectContext(ctx.Request().Context(), &items, `
		SELECT id, name, is_ranking, registration_deadline, NULLIF(article_url, '') AS article_url
		FROM event
		ORDER BY CASE WHEN registration_deadline IS NULL THEN 1 ELSE 0 END,
		registration_deadline ASC, display_order DESC, id DESC`)
	if err != nil && err != sql.ErrNoRows {
		return response.InternalError(ctx, "获取赛事信息失败")
	}
	return response.Success(ctx, items)
}

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
