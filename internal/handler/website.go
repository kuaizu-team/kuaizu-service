package handler

import (
	"strings"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
)

type websiteContentItem struct {
	ID           int    `db:"id" json:"id"`
	Title        string `db:"title" json:"title"`
	Content      string `db:"content" json:"content"`
	ArticleURL   string `db:"url" json:"articleUrl"`
	DisplayOrder int    `db:"display_order" json:"displayOrder"`
}

type websiteTeamRow struct {
	ID         int        `db:"id"`
	Nickname   *string    `db:"nickname"`
	Role       int        `db:"role"`
	SchoolName *string    `db:"school_name"`
	JoinDate   *time.Time `db:"join_date"`
	Intro      *string    `db:"intro"`
	ArticleURL *string    `db:"article_url"`
}

type websiteTeamItem struct {
	ID          int        `json:"id"`
	Nickname    *string    `json:"nickname"`
	Role        int        `json:"role"`
	SchoolName  string     `json:"schoolName"`
	SchoolNames []string   `json:"schoolNames"`
	JoinDate    *time.Time `json:"joinDate"`
	Intro       *string    `json:"intro"`
	ArticleURL  *string    `json:"articleUrl"`
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

// ListWebsiteTeam exposes only active public team members. Role-2 school
// names come from owned admin_school_relation rows; fully delegated admins are
// intentionally omitted. Role-3 admins continue to use admin_user.school_id.
func (s *Server) ListWebsiteTeam(ctx echo.Context) error {
	rows := make([]websiteTeamRow, 0)
	err := s.repo.DB().SelectContext(ctx.Request().Context(), &rows, `
		SELECT au.id, au.nickname, au.role, s.school_name, au.join_date, au.intro,
		       NULLIF(au.article_url, '') AS article_url
		FROM admin_user au
		LEFT JOIN admin_school_relation rel
		       ON au.role = ? AND rel.admin_user_id = au.id AND rel.is_owner = 1
		LEFT JOIN school s
		       ON s.id = CASE WHEN au.role = ? THEN rel.school_id ELSE au.school_id END
		WHERE au.status = 1
		  AND au.role IN (?, ?)
		  AND (au.role = ? OR rel.id IS NOT NULL)
		ORDER BY au.created_at DESC, au.id DESC, s.school_name ASC`,
		models.AdminRoleSchoolSuperAdmin,
		models.AdminRoleSchoolSuperAdmin,
		models.AdminRoleSchoolSuperAdmin,
		models.AdminRoleSchoolAdmin,
		models.AdminRoleSchoolAdmin,
	)
	if err != nil {
		return response.InternalError(ctx, "获取团队信息失败")
	}

	items := make([]websiteTeamItem, 0)
	indexByID := make(map[int]int)
	for _, row := range rows {
		index, exists := indexByID[row.ID]
		if !exists {
			index = len(items)
			indexByID[row.ID] = index
			items = append(items, websiteTeamItem{
				ID: row.ID, Nickname: row.Nickname, Role: row.Role,
				SchoolNames: make([]string, 0), JoinDate: row.JoinDate,
				Intro: row.Intro, ArticleURL: row.ArticleURL,
			})
		}
		if row.SchoolName != nil && strings.TrimSpace(*row.SchoolName) != "" {
			items[index].SchoolNames = append(items[index].SchoolNames, strings.TrimSpace(*row.SchoolName))
		}
	}
	for i := range items {
		items[i].SchoolName = strings.Join(items[i].SchoolNames, " · ")
	}
	return response.Success(ctx, items)
}
