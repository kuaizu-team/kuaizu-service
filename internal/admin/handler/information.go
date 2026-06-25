package handler

import (
	"database/sql"
	"errors"
	"net/url"
	"strconv"
	"strings"

	adminvo "github.com/kuaizu-team/kuaizu-service/internal/admin/vo"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
)

const informationForbiddenMessage = "only super admin can manage information content"

type informationContentRequest struct {
	Title        string  `json:"title"`
	Content      string  `json:"content"`
	URL          string  `json:"url"`
	Category     string  `json:"category"`
	DisplayOrder *int    `json:"displayOrder"`
	IsPublished  *bool   `json:"isPublished"`
	EventIDs     []int   `json:"eventIds"`
	CreatedAt    *string `json:"createdAt"`
}

// ListInformation handles GET /admin/information.
func (s *AdminServer) ListInformation(ctx echo.Context) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}

	page, _ := strconv.Atoi(ctx.QueryParam("page"))
	size, _ := strconv.Atoi(ctx.QueryParam("size"))
	params := repository.InformationContentListParams{Page: page, Size: size}

	if category := strings.TrimSpace(ctx.QueryParam("category")); category != "" {
		if !models.IsValidInformationCategory(category) {
			return response.BadRequest(ctx, "invalid category")
		}
		params.Category = &category
	}

	items, total, err := s.repo.InformationContent.AdminList(ctx.Request().Context(), params)
	if err != nil {
		return response.InternalError(ctx, "list information content failed")
	}

	list := make([]adminvo.AdminInformationContentVO, len(items))
	for i := range items {
		list[i] = *adminvo.NewAdminInformationContentVO(&items[i])
	}
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 10
	}

	return response.Success(ctx, map[string]interface{}{
		"list":  list,
		"total": total,
		"page":  page,
		"size":  size,
	})
}

// GetInformation handles GET /admin/information/:id.
func (s *AdminServer) GetInformation(ctx echo.Context) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}

	id, err := parseIDParam(ctx, "id", "information")
	if err != nil {
		return err
	}

	item, err := s.repo.InformationContent.AdminGetByID(ctx.Request().Context(), id)
	if err != nil {
		return response.InternalError(ctx, "get information content failed")
	}
	if item == nil {
		return response.NotFound(ctx, "information content not found")
	}

	return response.Success(ctx, adminvo.NewAdminInformationContentVO(item))
}

// CreateInformation handles POST /admin/information.
func (s *AdminServer) CreateInformation(ctx echo.Context) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}

	var req informationContentRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	item, err := buildInformationContent(req)
	if err != nil {
		return response.BadRequest(ctx, err.Error())
	}

	if err := s.repo.InformationContent.AdminCreate(ctx.Request().Context(), item); err != nil {
		return response.InternalError(ctx, "create information content failed")
	}

	created, err := s.repo.InformationContent.AdminGetByID(ctx.Request().Context(), item.ID)
	if err != nil {
		return response.InternalError(ctx, "get created information content failed")
	}
	return response.Success(ctx, adminvo.NewAdminInformationContentVO(created))
}

// UpdateInformation handles PUT /admin/information/:id.
func (s *AdminServer) UpdateInformation(ctx echo.Context) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}

	id, err := parseIDParam(ctx, "id", "information")
	if err != nil {
		return err
	}

	var req informationContentRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	item, err := buildInformationContent(req)
	if err != nil {
		return response.BadRequest(ctx, err.Error())
	}
	item.ID = id

	if err := s.repo.InformationContent.AdminUpdate(ctx.Request().Context(), item); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return response.NotFound(ctx, "information content not found")
		}
		return response.InternalError(ctx, "update information content failed")
	}

	updated, err := s.repo.InformationContent.AdminGetByID(ctx.Request().Context(), id)
	if err != nil {
		return response.InternalError(ctx, "get updated information content failed")
	}
	return response.Success(ctx, adminvo.NewAdminInformationContentVO(updated))
}

// DeleteInformation handles DELETE /admin/information/:id.
func (s *AdminServer) DeleteInformation(ctx echo.Context) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}

	id, err := parseIDParam(ctx, "id", "information")
	if err != nil {
		return err
	}

	if err := s.repo.InformationContent.AdminDelete(ctx.Request().Context(), id); err != nil {
		return response.InternalError(ctx, "delete information content failed")
	}
	return response.SuccessMessage(ctx, "operation succeeded")
}

func requireSuperAdmin(ctx echo.Context) error {
	if adminRole(ctx) != models.AdminRoleSuperAdmin {
		return response.Forbidden(ctx, informationForbiddenMessage)
	}
	return nil
}

func buildInformationEvents(category string, eventIDs []int) []models.Event {
	if category != models.InformationCategoryCampusEvent || len(eventIDs) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(eventIDs))
	events := make([]models.Event, 0, len(eventIDs))
	for _, id := range eventIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		events = append(events, models.Event{ID: id})
	}
	return events
}
func buildInformationContent(req informationContentRequest) (*models.InformationContent, error) {
	title := strings.TrimSpace(req.Title)
	content := strings.TrimSpace(req.Content)
	link := strings.TrimSpace(req.URL)
	category := strings.TrimSpace(req.Category)

	if title == "" {
		return nil, errors.New("title is required")
	}
	if len([]rune(title)) > 200 {
		return nil, errors.New("title must be at most 200 characters")
	}
	if content == "" {
		return nil, errors.New("content is required")
	}
	if len([]rune(content)) > 1000 {
		return nil, errors.New("content must be at most 1000 characters")
	}
	if link == "" {
		return nil, errors.New("url is required")
	}
	parsedURL, err := url.ParseRequestURI(link)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, errors.New("url is invalid")
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, errors.New("url scheme must be http or https")
	}
	if !models.IsValidInformationCategory(category) {
		return nil, errors.New("invalid category")
	}

	displayOrder := 0
	if req.DisplayOrder != nil {
		displayOrder = *req.DisplayOrder
	}

	isPublished := 1
	if req.IsPublished != nil && !*req.IsPublished {
		isPublished = 0
	}

	return &models.InformationContent{
		Title:        title,
		Content:      content,
		URL:          link,
		Category:     category,
		DisplayOrder: displayOrder,
		IsPublished:  isPublished,
		Events:       buildInformationEvents(category, req.EventIDs),
	}, nil
}
