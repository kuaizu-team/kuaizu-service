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
		value, err := models.ParseEventDate(raw)
		if err != nil {
			return BadRequest(ctx, "invalid registrationDeadlineFrom")
		}
		params.RegistrationDeadlineFrom = &value
	}
	if raw := strings.TrimSpace(ctx.QueryParam("registrationDeadlineTo")); raw != "" {
		value, err := models.ParseEventDate(raw)
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
	event, projects, timeline, err := s.svc.Event.GetEvent(ctx.Request().Context(), id)
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
		"timeline": timeline,
	})
}

// ListEventTimeline handles GET /events/:id/timeline without recording another PV.
func (s *Server) ListEventTimeline(ctx echo.Context) error {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil || id <= 0 {
		return BadRequest(ctx, "invalid event id")
	}
	items, err := s.svc.Event.ListEventTimeline(ctx.Request().Context(), id)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return Success(ctx, items)
}

type infoCenterEventView string

const (
	infoCenterEventViewRecommend infoCenterEventView = "recommend"
	infoCenterEventViewSupport   infoCenterEventView = "support"
)

func parseInfoCenterEventView(raw string) (infoCenterEventView, bool) {
	view := infoCenterEventView(strings.TrimSpace(raw))
	switch view {
	case infoCenterEventViewRecommend, infoCenterEventViewSupport:
		return view, true
	default:
		return "", false
	}
}

func infoCenterSchoolEventParams(view infoCenterEventView, schoolID int, now time.Time) repository.EventListParams {
	businessNow := now.In(time.FixedZone("Asia/Shanghai", 8*60*60))
	deadlineFrom, _ := models.ParseEventDate(businessNow.Format("2006-01-02"))
	size := 100
	if view == infoCenterEventViewRecommend {
		size = 1
	}
	return repository.EventListParams{
		Page:                     1,
		Size:                     size,
		RegistrationDeadlineFrom: &deadlineFrom,
		SchoolIDs:                []int{schoolID},
		SchoolOnly:               true,
	}
}

func (s *Server) ListInfoCenterEvents(ctx echo.Context) error {
	rawView := strings.TrimSpace(ctx.QueryParam("view"))
	if rawView != "" {
		view, ok := parseInfoCenterEventView(rawView)
		if !ok {
			return BadRequest(ctx, "invalid view")
		}
		user, err := s.repo.User.GetByID(ctx.Request().Context(), GetUserID(ctx))
		if err != nil {
			return InternalError(ctx, "get current user failed")
		}
		if user == nil {
			return NotFound(ctx, "user not found")
		}

		events := make([]models.Event, 0)
		if user.SchoolID != nil {
			params := infoCenterSchoolEventParams(view, *user.SchoolID, time.Now())
			result, err := s.svc.Event.ListEvents(ctx.Request().Context(), params)
			if err != nil {
				return mapServiceError(ctx, err)
			}
			events = append(events, result.List...)
			if view == infoCenterEventViewSupport {
				for page := 2; page <= result.TotalPages; page++ {
					params.Page = page
					next, err := s.svc.Event.ListEvents(ctx.Request().Context(), params)
					if err != nil {
						return mapServiceError(ctx, err)
					}
					events = append(events, next.List...)
				}
			}
		}
		return writeInfoCenterEvents(ctx, events)
	}

	limit, _ := strconv.Atoi(ctx.QueryParam("limit"))
	events, err := s.svc.Event.ListTimeline(ctx.Request().Context(), limit)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return writeInfoCenterEvents(ctx, events)
}

func writeInfoCenterEvents(ctx echo.Context, events []models.Event) error {
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
		Name:             strings.TrimSpace(req.Name),
		DisplayOrder:     0,
		AllowCrossSchool: 1,
		AllowCrossMajor:  1,
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
		t, err := models.ParseEventDate(strings.TrimSpace(*req.RegistrationDeadline))
		if err != nil {
			return nil, err
		}
		event.RegistrationDeadline = &t
	}
	return event, nil
}
