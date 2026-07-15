package handler

import (
	"database/sql"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
	"strings"
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

// GetWebsiteOverview returns the homepage highlights in one request.
func (s *Server) GetWebsiteOverview(ctx echo.Context) error {
	requestCtx := ctx.Request().Context()
	loadContent := func(category string) ([]websiteContentItem, error) {
		items := make([]websiteContentItem, 0, 3)
		err := s.repo.DB().SelectContext(requestCtx, &items, `
			SELECT id, title, content, url, display_order
			FROM information_content
			WHERE category = ? AND is_published = 1
			ORDER BY display_order DESC, created_at DESC, id DESC
			LIMIT 3`, category)
		return items, err
	}

	podcast, err := loadContent("kuaizu_talking")
	if err != nil {
		return response.InternalError(ctx, "获取播客精选失败")
	}
	projects, err := loadContent("campus_project")
	if err != nil {
		return response.InternalError(ctx, "获取项目精选失败")
	}
	talent, err := loadContent("talent")
	if err != nil {
		return response.InternalError(ctx, "获取人才精选失败")
	}

	events := make([]websiteEventItem, 0, 3)
	err = s.repo.DB().SelectContext(requestCtx, &events, `
		SELECT id, name, is_ranking, registration_deadline, NULLIF(article_url, '') AS article_url
		FROM event
		WHERE registration_deadline IS NULL
		   OR DATE_ADD(registration_deadline, INTERVAL 1 DAY) > NOW()
		ORDER BY CASE WHEN registration_deadline IS NULL THEN 1 ELSE 0 END,
		registration_deadline ASC, display_order DESC, id DESC
		LIMIT 3`)
	if err != nil {
		return response.InternalError(ctx, "获取赛事精选失败")
	}

	team := make([]struct {
		ID         int        `db:"id" json:"id"`
		Nickname   *string    `db:"nickname" json:"nickname"`
		Role       int        `db:"role" json:"role"`
		SchoolName *string    `db:"school_name" json:"schoolName"`
		JoinDate   *time.Time `db:"join_date" json:"joinDate"`
		Intro      *string    `db:"intro" json:"intro"`
		ArticleURL *string    `db:"article_url" json:"articleUrl"`
	}, 0, 5)
	err = s.repo.DB().SelectContext(requestCtx, &team, `
		SELECT au.id, au.nickname, au.role, s.school_name, au.join_date, au.intro,
		       NULLIF(au.article_url, '') AS article_url
		FROM admin_user au
		LEFT JOIN school s ON s.id = au.school_id
		WHERE au.status = 1 AND au.role = ?
		ORDER BY CASE WHEN au.article_url IS NULL OR au.article_url = '' THEN 1 ELSE 0 END,
		         au.join_date ASC, au.created_at ASC
		LIMIT 5`, models.AdminRoleSchoolSuperAdmin)
	if err != nil {
		return response.InternalError(ctx, "获取团队精选失败")
	}

	return response.Success(ctx, map[string]interface{}{
		"podcast": podcast, "team": team, "events": events,
		"projects": projects, "talent": talent,
	})
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
		schoolNames := make([]string, 0, len(a.Schools))
		if a.Role == models.AdminRoleSchoolSuperAdmin {
			for _, school := range a.Schools {
				if school.IsOwner {
					schoolNames = append(schoolNames, school.SchoolName)
				}
			}
			// A fully delegated administrator no longer appears on the public team page.
			if len(schoolNames) == 0 {
				continue
			}
		} else if a.SchoolName != nil && *a.SchoolName != "" {
			schoolNames = append(schoolNames, *a.SchoolName)
		}
		result = append(result, map[string]interface{}{
			"id": a.ID, "nickname": a.Nickname, "role": a.Role,
			"schoolName": strings.Join(schoolNames, " · "), "schoolNames": schoolNames,
			"joinDate": a.JoinDate, "intro": a.Intro, "articleUrl": a.ArticleURL,
		})
	}
	return response.Success(ctx, result)
}
