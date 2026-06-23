package handler

import (
	"database/sql"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
)

type adminProjectRecommendationRequest struct {
	ProjectID    int     `json:"projectId"`
	DisplayOrder *int    `json:"displayOrder"`
	IsVisible    *bool   `json:"isVisible"`
	IsFeatured   *bool   `json:"isFeatured"`
	InterviewURL *string `json:"interviewUrl"`
}

type adminProjectRecommendationVO struct {
	ID             int            `json:"id"`
	ProjectID      int            `json:"projectId"`
	DisplayOrder   int            `json:"displayOrder"`
	IsVisible      bool           `json:"isVisible"`
	IsFeatured     bool           `json:"isFeatured"`
	InterviewURL   *string        `json:"interviewUrl"`
	IsFromMySchool bool           `json:"isFromMySchool"`
	CreatedAt      interface{}    `json:"createdAt"`
	UpdatedAt      interface{}    `json:"updatedAt"`
	Project        *api.ProjectVO `json:"project"`
}

type adminArticleRecommendationVO struct {
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
type adminArticleRecommendationRequest struct {
	Title        string  `json:"title"`
	Description  *string `json:"description"`
	ArticleURL   string  `json:"articleUrl"`
	DisplayOrder *int    `json:"displayOrder"`
	IsVisible    *bool   `json:"isVisible"`
	IsFeatured   *bool   `json:"isFeatured"`
}

func (s *AdminServer) ListProjectRecommendations(ctx echo.Context) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}
	limit, _ := strconv.Atoi(ctx.QueryParam("limit"))
	items, err := s.repo.Recommendation.ListProjects(ctx.Request().Context(), repository.ProjectRecommendationListParams{Limit: limit})
	if err != nil {
		return response.InternalError(ctx, "list project recommendations failed")
	}
	return response.Success(ctx, map[string]interface{}{"list": adminProjectRecommendationVOs(items)})
}

func (s *AdminServer) CreateProjectRecommendation(ctx echo.Context) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}
	var req adminProjectRecommendationRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	item, err := buildProjectRecommendation(req)
	if err != nil {
		return response.BadRequest(ctx, err.Error())
	}
	if err := s.repo.Recommendation.UpsertProject(ctx.Request().Context(), item); err != nil {
		return response.InternalError(ctx, "save project recommendation failed")
	}
	items, err := s.repo.Recommendation.ListProjects(ctx.Request().Context(), repository.ProjectRecommendationListParams{Limit: 100})
	if err != nil {
		return response.InternalError(ctx, "get project recommendation failed")
	}
	for i := range items {
		if items[i].ProjectID == item.ProjectID {
			return response.Success(ctx, adminProjectRecommendationVOs([]models.ProjectRecommendation{items[i]})[0])
		}
	}
	return response.Success(ctx, nil)
}

func (s *AdminServer) UpdateProjectRecommendation(ctx echo.Context) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}
	id, err := parseIDParam(ctx, "id", "project recommendation")
	if err != nil {
		return err
	}
	var req adminProjectRecommendationRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	item, err := buildProjectRecommendation(req)
	if err != nil {
		return response.BadRequest(ctx, err.Error())
	}
	item.ID = id
	if err := s.repo.Recommendation.UpdateProject(ctx.Request().Context(), item); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return response.NotFound(ctx, "project recommendation not found")
		}
		return response.InternalError(ctx, "update project recommendation failed")
	}
	updated, err := s.repo.Recommendation.GetProjectRecommendation(ctx.Request().Context(), id)
	if err != nil {
		return response.InternalError(ctx, "get project recommendation failed")
	}
	return response.Success(ctx, adminProjectRecommendationVOs([]models.ProjectRecommendation{*updated})[0])
}

func (s *AdminServer) DeleteProjectRecommendation(ctx echo.Context) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}
	id, err := parseIDParam(ctx, "id", "project recommendation")
	if err != nil {
		return err
	}
	if err := s.repo.Recommendation.DeleteProject(ctx.Request().Context(), id); err != nil {
		return response.InternalError(ctx, "delete project recommendation failed")
	}
	return response.SuccessMessage(ctx, "operation succeeded")
}

func (s *AdminServer) ListPodcastRecommendations(ctx echo.Context) error {
	return s.listArticleRecommendations(ctx, "podcasts")
}
func (s *AdminServer) GetPodcastRecommendation(ctx echo.Context) error {
	return s.getArticleRecommendation(ctx, "podcasts")
}
func (s *AdminServer) CreatePodcastRecommendation(ctx echo.Context) error {
	return s.createArticleRecommendation(ctx, "podcasts")
}
func (s *AdminServer) UpdatePodcastRecommendation(ctx echo.Context) error {
	return s.updateArticleRecommendation(ctx, "podcasts")
}
func (s *AdminServer) DeletePodcastRecommendation(ctx echo.Context) error {
	return s.deleteArticleRecommendation(ctx, "podcasts")
}

func (s *AdminServer) ListNewsRecommendations(ctx echo.Context) error {
	return s.listArticleRecommendations(ctx, "news")
}
func (s *AdminServer) GetNewsRecommendation(ctx echo.Context) error {
	return s.getArticleRecommendation(ctx, "news")
}
func (s *AdminServer) CreateNewsRecommendation(ctx echo.Context) error {
	return s.createArticleRecommendation(ctx, "news")
}
func (s *AdminServer) UpdateNewsRecommendation(ctx echo.Context) error {
	return s.updateArticleRecommendation(ctx, "news")
}
func (s *AdminServer) DeleteNewsRecommendation(ctx echo.Context) error {
	return s.deleteArticleRecommendation(ctx, "news")
}

func (s *AdminServer) listArticleRecommendations(ctx echo.Context, kind string) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}
	limit, _ := strconv.Atoi(ctx.QueryParam("limit"))
	items, err := s.repo.Recommendation.ListArticles(ctx.Request().Context(), kind, repository.ArticleRecommendationListParams{Limit: limit})
	if err != nil {
		return response.InternalError(ctx, "list article recommendations failed")
	}
	return response.Success(ctx, map[string]interface{}{"list": adminArticleRecommendationVOs(items)})
}

func (s *AdminServer) getArticleRecommendation(ctx echo.Context, kind string) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}
	id, err := parseIDParam(ctx, "id", "article recommendation")
	if err != nil {
		return err
	}
	item, err := s.repo.Recommendation.GetArticle(ctx.Request().Context(), kind, id)
	if err != nil {
		return response.InternalError(ctx, "get article recommendation failed")
	}
	if item == nil {
		return response.NotFound(ctx, "article recommendation not found")
	}
	return response.Success(ctx, adminArticleRecommendationVOs([]models.ArticleRecommendation{*item})[0])
}

func (s *AdminServer) createArticleRecommendation(ctx echo.Context, kind string) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}
	var req adminArticleRecommendationRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	item, err := buildArticleRecommendation(req)
	if err != nil {
		return response.BadRequest(ctx, err.Error())
	}
	if err := s.repo.Recommendation.CreateArticle(ctx.Request().Context(), kind, item); err != nil {
		return response.InternalError(ctx, "create article recommendation failed")
	}
	created, err := s.repo.Recommendation.GetArticle(ctx.Request().Context(), kind, item.ID)
	if err != nil {
		return response.InternalError(ctx, "get article recommendation failed")
	}
	return response.Success(ctx, adminArticleRecommendationVOs([]models.ArticleRecommendation{*created})[0])
}

func (s *AdminServer) updateArticleRecommendation(ctx echo.Context, kind string) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}
	id, err := parseIDParam(ctx, "id", "article recommendation")
	if err != nil {
		return err
	}
	var req adminArticleRecommendationRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	item, err := buildArticleRecommendation(req)
	if err != nil {
		return response.BadRequest(ctx, err.Error())
	}
	item.ID = id
	if err := s.repo.Recommendation.UpdateArticle(ctx.Request().Context(), kind, item); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return response.NotFound(ctx, "article recommendation not found")
		}
		return response.InternalError(ctx, "update article recommendation failed")
	}
	updated, err := s.repo.Recommendation.GetArticle(ctx.Request().Context(), kind, id)
	if err != nil {
		return response.InternalError(ctx, "get article recommendation failed")
	}
	return response.Success(ctx, adminArticleRecommendationVOs([]models.ArticleRecommendation{*updated})[0])
}

func (s *AdminServer) deleteArticleRecommendation(ctx echo.Context, kind string) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}
	id, err := parseIDParam(ctx, "id", "article recommendation")
	if err != nil {
		return err
	}
	if err := s.repo.Recommendation.DeleteArticle(ctx.Request().Context(), kind, id); err != nil {
		return response.InternalError(ctx, "delete article recommendation failed")
	}
	return response.SuccessMessage(ctx, "operation succeeded")
}

func buildProjectRecommendation(req adminProjectRecommendationRequest) (*models.ProjectRecommendation, error) {
	if req.ProjectID <= 0 {
		return nil, errors.New("projectId is required")
	}
	item := &models.ProjectRecommendation{ProjectID: req.ProjectID, IsVisible: 1}
	if req.DisplayOrder != nil {
		item.DisplayOrder = *req.DisplayOrder
	}
	if req.IsVisible != nil {
		item.IsVisible = models.BoolInt(*req.IsVisible)
	}
	if req.IsFeatured != nil {
		item.IsFeatured = models.BoolInt(*req.IsFeatured)
	}
	if req.InterviewURL != nil {
		value := strings.TrimSpace(*req.InterviewURL)
		if value != "" {
			parsedURL, err := url.ParseRequestURI(value)
			if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
				return nil, errors.New("interviewUrl is invalid")
			}
			item.InterviewURL = &value
		}
	}
	return item, nil
}

func buildArticleRecommendation(req adminArticleRecommendationRequest) (*models.ArticleRecommendation, error) {
	title := strings.TrimSpace(req.Title)
	articleURL := strings.TrimSpace(req.ArticleURL)
	if title == "" {
		return nil, errors.New("title is required")
	}
	if len([]rune(title)) > 200 {
		return nil, errors.New("title must be at most 200 characters")
	}
	if articleURL == "" {
		return nil, errors.New("articleUrl is required")
	}
	parsedURL, err := url.ParseRequestURI(articleURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return nil, errors.New("articleUrl is invalid")
	}
	var description *string
	if req.Description != nil {
		value := strings.TrimSpace(*req.Description)
		if value != "" {
			description = &value
		}
	}
	item := &models.ArticleRecommendation{Title: title, Description: description, ArticleURL: articleURL, IsVisible: 1}
	if req.DisplayOrder != nil {
		item.DisplayOrder = *req.DisplayOrder
	}
	if req.IsVisible != nil {
		item.IsVisible = models.BoolInt(*req.IsVisible)
	}
	if req.IsFeatured != nil {
		item.IsFeatured = models.BoolInt(*req.IsFeatured)
	}
	return item, nil
}
func adminProjectRecommendationVOs(items []models.ProjectRecommendation) []adminProjectRecommendationVO {
	list := make([]adminProjectRecommendationVO, 0, len(items))
	for i := range items {
		var project *api.ProjectVO
		if items[i].Project != nil {
			project = items[i].Project.ToVO()
		}
		list = append(list, adminProjectRecommendationVO{
			ID:             items[i].ID,
			ProjectID:      items[i].ProjectID,
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

func adminArticleRecommendationVOs(items []models.ArticleRecommendation) []adminArticleRecommendationVO {
	list := make([]adminArticleRecommendationVO, 0, len(items))
	for i := range items {
		list = append(list, adminArticleRecommendationVO{
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
