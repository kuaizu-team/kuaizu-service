package handler

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	adminvo "github.com/kuaizu-team/kuaizu-service/internal/admin/vo"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
)

type adminEventRequest struct {
	Name                 string  `json:"name"`
	IsRanking            *bool   `json:"isRanking"`
	RegistrationDeadline *string `json:"registrationDeadline"`
	ArticleURL           *string `json:"articleUrl"`
	DisplayOrder         *int    `json:"displayOrder"`
	CreatedAt            *string `json:"createdAt"`
	Level                *string `json:"level"`
	Summary              *string `json:"summary"`
	SchoolID             *int    `json:"schoolId"`
}

type adminEventMergeRequest struct {
	TargetEventID int `json:"targetEventId"`
}

func (s *AdminServer) ListEvents(ctx echo.Context) error {
	page, _ := strconv.Atoi(ctx.QueryParam("page"))
	size, _ := strconv.Atoi(ctx.QueryParam("size"))
	keyword := strings.TrimSpace(ctx.QueryParam("keyword"))
	var keywordPtr *string
	if keyword != "" {
		keywordPtr = &keyword
	}
	result, err := s.svc.Event.ListEvents(ctx.Request().Context(), repository.EventListParams{
		Page: page, Size: size, Keyword: keywordPtr,
	})
	if err != nil {
		return mapServiceError(ctx, err)
	}
	list := make([]adminvo.AdminEventVO, len(result.List))
	for i := range result.List {
		list[i] = *adminvo.NewAdminEventVO(&result.List[i])
	}
	return response.Success(ctx, map[string]interface{}{
		"list": list, "total": result.Total, "page": result.Page, "size": result.Size,
	})
}

func (s *AdminServer) CreateEvent(ctx echo.Context) error {
	var req adminEventRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	event, err := buildAdminEventModel(req, adminRole(ctx), adminSchoolID(ctx))
	if err != nil {
		return response.BadRequest(ctx, err.Error())
	}
	created, err := s.svc.Event.CreateEvent(ctx.Request().Context(), event)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return response.Success(ctx, adminvo.NewAdminEventVO(created))
}

func (s *AdminServer) UpdateEvent(ctx echo.Context) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}
	id, err := parseIDParam(ctx, "id", "event")
	if err != nil {
		return err
	}
	var req adminEventRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	event, err := buildAdminEventModel(req, adminRole(ctx), adminSchoolID(ctx))
	if err != nil {
		return response.BadRequest(ctx, err.Error())
	}
	event.ID = id
	updated, err := s.svc.Event.UpdateEvent(ctx.Request().Context(), event)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return response.NotFound(ctx, "event not found")
		}
		return mapServiceError(ctx, err)
	}
	return response.Success(ctx, adminvo.NewAdminEventVO(updated))
}

func (s *AdminServer) DeleteEvent(ctx echo.Context) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}
	id, err := parseIDParam(ctx, "id", "event")
	if err != nil {
		return err
	}
	if err := s.svc.Event.DeleteEvent(ctx.Request().Context(), id); err != nil {
		return mapServiceError(ctx, err)
	}
	return response.SuccessMessage(ctx, "operation succeeded")
}

func (s *AdminServer) MergeEvent(ctx echo.Context) error {
	id, err := parseIDParam(ctx, "id", "event")
	if err != nil {
		return err
	}
	var req adminEventMergeRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	source, err := s.repo.Event.GetByID(ctx.Request().Context(), id)
	if err != nil || source == nil {
		return response.NotFound(ctx, "event not found")
	}
	target, err := s.repo.Event.GetByID(ctx.Request().Context(), req.TargetEventID)
	if err != nil || target == nil {
		return response.NotFound(ctx, "target event not found")
	}
	levelRank := map[string]int{"school": 1, "regional": 2, "national": 3}
	if source.Level == nil || target.Level == nil || levelRank[*target.Level] <= levelRank[*source.Level] {
		return response.BadRequest(ctx, "events can only be merged upward: school to regional to national")
	}
	merged, err := s.svc.Event.MergeEvent(ctx.Request().Context(), id, req.TargetEventID)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return response.Success(ctx, adminvo.NewAdminEventVO(merged))
}

func buildAdminEventModel(req adminEventRequest, role int, adminSchoolID *int) (*models.Event, error) {
	event := &models.Event{Name: strings.TrimSpace(req.Name)}
	if req.Level != nil && strings.TrimSpace(*req.Level) != "" {
		level := strings.TrimSpace(*req.Level)
		if level != "national" && level != "regional" && level != "school" {
			return nil, errors.New("level must be national, regional, or school")
		}
		event.Level = &level
		if level == "school" {
			if role == models.AdminRoleSuperAdmin {
				if req.SchoolID == nil || *req.SchoolID <= 0 {
					return nil, errors.New("schoolId is required for school-level events")
				}
				event.SchoolID = req.SchoolID
			} else {
				if adminSchoolID == nil {
					return nil, errors.New("current admin is not associated with a school")
				}
				event.SchoolID = adminSchoolID
			}
		}
	}
	if req.Summary != nil {
		value := strings.TrimSpace(*req.Summary)
		if value != "" {
			event.Summary = &value
		}
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
	if req.CreatedAt != nil && strings.TrimSpace(*req.CreatedAt) != "" {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(*req.CreatedAt))
		if err != nil {
			return nil, err
		}
		event.CreatedAt = t
	}
	return event, nil
}
