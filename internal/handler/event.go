package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/labstack/echo/v4"
)

type eventRequest struct {
	Name                 string  `json:"name"`
	IsRanking            *bool   `json:"isRanking"`
	RegistrationDeadline *string `json:"registrationDeadline"`
	ArticleURL           *string `json:"articleUrl"`
	DisplayOrder         *int    `json:"displayOrder"`
}

func (s *Server) ListEvents(ctx echo.Context) error {
	page, _ := strconv.Atoi(ctx.QueryParam("page"))
	size, _ := strconv.Atoi(ctx.QueryParam("size"))
	keyword := strings.TrimSpace(ctx.QueryParam("keyword"))
	var keywordPtr *string
	if keyword != "" {
		keywordPtr = &keyword
	}
	params := repository.EventListParams{Page: page, Size: size, Keyword: keywordPtr}
	if raw := strings.TrimSpace(ctx.QueryParam("isRanking")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return BadRequest(ctx, "invalid isRanking")
		}
		params.IsRanking = &value
	}
	if raw := strings.TrimSpace(ctx.QueryParam("registrationDeadlineFrom")); raw != "" {
		value, err := time.Parse("2006-01-02", raw)
		if err != nil {
			return BadRequest(ctx, "invalid registrationDeadlineFrom")
		}
		params.RegistrationDeadlineFrom = &value
	}
	if raw := strings.TrimSpace(ctx.QueryParam("registrationDeadlineTo")); raw != "" {
		value, err := time.Parse("2006-01-02", raw)
		if err != nil {
			return BadRequest(ctx, "invalid registrationDeadlineTo")
		}
		params.RegistrationDeadlineTo = &value
	}
	result, err := s.svc.Event.ListEvents(ctx.Request().Context(), params)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	list := make([]api.EventVO, len(result.List))
	for i := range result.List {
		list[i] = result.List[i].ToVO()
	}
	return Success(ctx, map[string]interface{}{
		"list":       list,
		"total":      result.Total,
		"page":       result.Page,
		"size":       result.Size,
		"totalPages": result.TotalPages,
	})
}

func (s *Server) CreateEvent(ctx echo.Context) error {
	var req eventRequest
	if err := ctx.Bind(&req); err != nil {
		return BadRequest(ctx, "invalid request body")
	}
	event, err := buildEventModel(req)
	if err != nil {
		return BadRequest(ctx, err.Error())
	}
	created, err := s.svc.Event.CreateEvent(ctx.Request().Context(), event)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return Success(ctx, created.ToVO())
}

func (s *Server) GetEvent(ctx echo.Context) error {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil || id <= 0 {
		return BadRequest(ctx, "invalid event id")
	}
	event, projects, err := s.svc.Event.GetEvent(ctx.Request().Context(), id)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	projectVOs := make([]api.ProjectVO, len(projects))
	for i := range projects {
		projectVOs[i] = *projects[i].ToVO()
	}
	return Success(ctx, map[string]interface{}{
		"event":    event.ToVO(),
		"projects": projectVOs,
	})
}

func (s *Server) ListInfoCenterEvents(ctx echo.Context) error {
	limit, _ := strconv.Atoi(ctx.QueryParam("limit"))
	events, err := s.svc.Event.ListTimeline(ctx.Request().Context(), limit)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	list := make([]api.EventVO, len(events))
	for i := range events {
		list[i] = events[i].ToVO()
	}
	return ctx.JSON(http.StatusOK, map[string]interface{}{
		"code":    0,
		"message": "success",
		"data":    list,
	})
}

func buildEventModel(req eventRequest) (*models.Event, error) {
	event := &models.Event{
		Name:         strings.TrimSpace(req.Name),
		DisplayOrder: 0,
	}
	if req.IsRanking != nil && *req.IsRanking {
		event.IsRanking = 1
	}
	if req.DisplayOrder != nil {
		event.DisplayOrder = *req.DisplayOrder
	}
	if req.ArticleURL != nil {
		value := strings.TrimSpace(*req.ArticleURL)
		if value != "" {
			event.ArticleURL = &value
		}
	}
	if req.RegistrationDeadline != nil && strings.TrimSpace(*req.RegistrationDeadline) != "" {
		t, err := time.Parse("2006-01-02", strings.TrimSpace(*req.RegistrationDeadline))
		if err != nil {
			return nil, err
		}
		event.RegistrationDeadline = &t
	}
	return event, nil
}
