package handler

import (
	"database/sql"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	adminvo "github.com/kuaizu-team/kuaizu-service/internal/admin/vo"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
)

type roadmapRequest struct {
	Date    string `json:"date"`
	Title   string `json:"title"`
	Content string `json:"content"`
	Link    string `json:"link"`
}

type sendVersionUpdateRequest struct {
	RoadmapID int `json:"roadmap_id"`
}

// ListRoadmaps handles GET /admin/roadmap.
func (s *AdminServer) ListRoadmaps(ctx echo.Context) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}

	page, _ := strconv.Atoi(ctx.QueryParam("page"))
	size, _ := strconv.Atoi(ctx.QueryParam("size"))
	params := repository.RoadmapListParams{Page: page, Size: size}

	items, total, err := s.repo.Roadmap.AdminList(ctx.Request().Context(), params)
	if err != nil {
		return response.InternalError(ctx, "list roadmap failed")
	}
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 10
	}

	list := make([]adminvo.AdminRoadmapVO, len(items))
	for i := range items {
		list[i] = *adminvo.NewAdminRoadmapVO(&items[i])
	}

	return response.Success(ctx, map[string]interface{}{
		"list":  list,
		"total": total,
		"page":  page,
		"size":  size,
	})
}

// GetRoadmap handles GET /admin/roadmap/:id.
func (s *AdminServer) GetRoadmap(ctx echo.Context) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}

	id, err := parseIDParam(ctx, "id", "roadmap")
	if err != nil {
		return err
	}

	item, err := s.repo.Roadmap.AdminGetByID(ctx.Request().Context(), id)
	if err != nil {
		return response.InternalError(ctx, "get roadmap failed")
	}
	if item == nil {
		return response.NotFound(ctx, "roadmap item not found")
	}

	return response.Success(ctx, adminvo.NewAdminRoadmapVO(item))
}

// CreateRoadmap handles POST /admin/roadmap.
func (s *AdminServer) CreateRoadmap(ctx echo.Context) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}

	var req roadmapRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	item, err := buildRoadmap(req)
	if err != nil {
		return response.BadRequest(ctx, err.Error())
	}

	if err := s.repo.Roadmap.AdminCreate(ctx.Request().Context(), item); err != nil {
		return response.InternalError(ctx, "create roadmap failed")
	}

	created, err := s.repo.Roadmap.AdminGetByID(ctx.Request().Context(), item.ID)
	if err != nil {
		return response.InternalError(ctx, "get created roadmap failed")
	}
	return response.Success(ctx, adminvo.NewAdminRoadmapVO(created))
}

// UpdateRoadmap handles PUT /admin/roadmap/:id.
func (s *AdminServer) UpdateRoadmap(ctx echo.Context) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}

	id, err := parseIDParam(ctx, "id", "roadmap")
	if err != nil {
		return err
	}

	var req roadmapRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	item, err := buildRoadmap(req)
	if err != nil {
		return response.BadRequest(ctx, err.Error())
	}
	item.ID = id

	if err := s.repo.Roadmap.AdminUpdate(ctx.Request().Context(), item); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return response.NotFound(ctx, "roadmap item not found")
		}
		return response.InternalError(ctx, "update roadmap failed")
	}

	updated, err := s.repo.Roadmap.AdminGetByID(ctx.Request().Context(), id)
	if err != nil {
		return response.InternalError(ctx, "get updated roadmap failed")
	}
	return response.Success(ctx, adminvo.NewAdminRoadmapVO(updated))
}

// DeleteRoadmap handles DELETE /admin/roadmap/:id.
func (s *AdminServer) DeleteRoadmap(ctx echo.Context) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}

	id, err := parseIDParam(ctx, "id", "roadmap")
	if err != nil {
		return err
	}

	if err := s.repo.Roadmap.AdminDelete(ctx.Request().Context(), id); err != nil {
		return response.InternalError(ctx, "delete roadmap failed")
	}
	return response.SuccessMessage(ctx, "operation succeeded")
}

// SendVersionUpdate handles POST /admin/send-version-update.
func (s *AdminServer) SendVersionUpdate(ctx echo.Context) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}

	var req sendVersionUpdateRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}

	result, err := s.svc.Message.StartVersionUpdateBroadcast(ctx.Request().Context(), req.RoadmapID)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return response.Success(ctx, result)
}

func buildRoadmap(req roadmapRequest) (*models.Roadmap, error) {
	dateText := strings.TrimSpace(req.Date)
	title := strings.TrimSpace(req.Title)
	content := strings.TrimSpace(req.Content)
	linkText := strings.TrimSpace(req.Link)

	if dateText == "" {
		return nil, errors.New("date is required")
	}
	date, err := time.ParseInLocation("2006-01-02", dateText, time.Local)
	if err != nil {
		return nil, errors.New("date must use YYYY-MM-DD format")
	}
	if title == "" {
		return nil, errors.New("title is required")
	}
	if len([]rune(title)) > 100 {
		return nil, errors.New("title must be at most 100 characters")
	}
	if content == "" {
		return nil, errors.New("content is required")
	}
	if len([]rune(content)) > 2000 {
		return nil, errors.New("content must be at most 2000 characters")
	}

	var link *string
	if linkText != "" {
		parsedURL, err := url.ParseRequestURI(linkText)
		if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
			return nil, errors.New("link is invalid")
		}
		if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
			return nil, errors.New("link scheme must be http or https")
		}
		link = &linkText
	}

	return &models.Roadmap{
		Date:    date,
		Title:   title,
		Content: content,
		Link:    link,
	}, nil
}
