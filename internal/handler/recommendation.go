package handler

import (
	"log"
	"strconv"

	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/labstack/echo/v4"
)

type projectRecommendationVO struct {
	ID             int            `json:"id"`
	ProjectID      int            `json:"projectId"`
	Description    *string        `json:"description"`
	DisplayOrder   int            `json:"displayOrder"`
	IsVisible      bool           `json:"isVisible"`
	IsFeatured     bool           `json:"isFeatured"`
	InterviewURL   *string        `json:"interviewUrl"`
	IsFromMySchool bool           `json:"isFromMySchool"`
	CreatedAt      interface{}    `json:"createdAt"`
	UpdatedAt      interface{}    `json:"updatedAt"`
	Project        *api.ProjectVO `json:"project"`
}

type articleRecommendationVO struct {
	ID           int         `json:"id"`
	Title        string      `json:"title"`
	Description  *string     `json:"description"`
	ArticleURL   string      `json:"articleUrl"`
	DisplayOrder int         `json:"displayOrder"`
	IsVisible    bool        `json:"isVisible"`
	IsFeatured   bool        `json:"isFeatured"`
	CreatedAt    interface{} `json:"createdAt"`
	UpdatedAt    interface{} `json:"updatedAt"`
}

func (s *Server) ListRecommendationProjects(ctx echo.Context) error {
	userID := GetOptionalUserID(ctx)
	var schoolID *int
	if userID > 0 {
		user, err := s.repo.User.GetByID(ctx.Request().Context(), userID)
		if err != nil {
			log.Printf("ListRecommendationProjects user lookup error: %v", err)
			return InternalError(ctx, "get current user failed")
		}
		if user != nil {
			schoolID = user.SchoolID
		}
	}
	limit, _ := strconv.Atoi(ctx.QueryParam("limit"))
	items, err := s.repo.Recommendation.ListProjects(ctx.Request().Context(), repository.ProjectRecommendationListParams{
		VisibleOnly: true,
		SchoolID:    schoolID,
		Limit:       limit,
	})
	if err != nil {
		log.Printf("ListRecommendationProjects error: %v", err)
		return InternalError(ctx, "get recommendation projects failed")
	}
	return Success(ctx, map[string]interface{}{"list": projectRecommendationVOs(items)})
}

func (s *Server) GetFeaturedRecommendationProject(ctx echo.Context) error {
	items, err := s.repo.Recommendation.ListProjects(ctx.Request().Context(), repository.ProjectRecommendationListParams{
		VisibleOnly:  true,
		FeaturedOnly: true,
		Limit:        1,
	})
	if err != nil {
		log.Printf("GetFeaturedRecommendationProject error: %v", err)
		return InternalError(ctx, "get featured project failed")
	}
	if len(items) == 0 {
		return Success(ctx, nil)
	}
	return Success(ctx, projectRecommendationVOs(items)[0])
}

func (s *Server) ListRecommendationPodcasts(ctx echo.Context) error {
	return s.listRecommendationArticles(ctx, "podcasts")
}

func (s *Server) ListRecommendationNews(ctx echo.Context) error {
	return s.listRecommendationArticles(ctx, "news")
}

func (s *Server) listRecommendationArticles(ctx echo.Context, kind string) error {
	limit, _ := strconv.Atoi(ctx.QueryParam("limit"))
	items, err := s.repo.Recommendation.ListArticles(ctx.Request().Context(), kind, repository.ArticleRecommendationListParams{VisibleOnly: true, Limit: limit})
	if err != nil {
		log.Printf("ListRecommendationArticles(%s) error: %v", kind, err)
		return InternalError(ctx, "get recommendations failed")
	}
	return Success(ctx, map[string]interface{}{"list": articleRecommendationVOs(items)})
}

func projectRecommendationVOs(items []models.ProjectRecommendation) []projectRecommendationVO {
	list := make([]projectRecommendationVO, 0, len(items))
	for i := range items {
		var project *api.ProjectVO
		if items[i].Project != nil {
			project = items[i].Project.ToVO()
		}
		list = append(list, projectRecommendationVO{
			ID:             items[i].ID,
			ProjectID:      items[i].ProjectID,
			Description:    items[i].Description,
			DisplayOrder:   items[i].DisplayOrder,
			IsVisible:      items[i].IsVisible == 1,
			IsFeatured:     items[i].IsFeatured == 1,
			InterviewURL:   items[i].InterviewURL,
			IsFromMySchool: items[i].IsFromMySchool,
			CreatedAt:      items[i].CreatedAt,
			UpdatedAt:      items[i].UpdatedAt,
			Project:        project,
		})
	}
	return list
}

func articleRecommendationVOs(items []models.ArticleRecommendation) []articleRecommendationVO {
	list := make([]articleRecommendationVO, 0, len(items))
	for i := range items {
		list = append(list, articleRecommendationVO{
			ID:           items[i].ID,
			Title:        items[i].Title,
			Description:  items[i].Description,
			ArticleURL:   items[i].ArticleURL,
			DisplayOrder: items[i].DisplayOrder,
			IsVisible:    items[i].IsVisible == 1,
			IsFeatured:   items[i].IsFeatured == 1,
			CreatedAt:    items[i].CreatedAt,
			UpdatedAt:    items[i].UpdatedAt,
		})
	}
	return list
}
