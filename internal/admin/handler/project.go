package handler

import (
	"fmt"
	"strconv"

	adminvo "github.com/kuaizu-team/kuaizu-service/internal/admin/vo"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
)

// ListProjects handles GET /admin/projects
func (s *AdminServer) ListProjects(ctx echo.Context) error {
	page, _ := strconv.Atoi(ctx.QueryParam("page"))
	size, _ := strconv.Atoi(ctx.QueryParam("size"))

	params := repository.ListParams{
		Page: page,
		Size: size,
	}

	if v := ctx.QueryParam("status"); v != "" {
		status, err := strconv.Atoi(v)
		if err != nil {
			return response.BadRequest(ctx, "invalid status")
		}
		params.Status = &status
	}

	if v := ctx.QueryParam("keyword"); v != "" {
		params.Keyword = &v
	}

	result, err := s.svc.Project.ListProjects(ctx.Request().Context(), params)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	list := make([]adminvo.AdminProjectVO, len(result.List))
	for i := range result.List {
		list[i] = *adminvo.NewAdminProjectVO(&result.List[i])
	}

	return response.Success(ctx, map[string]interface{}{
		"list":  list,
		"total": result.Total,
		"page":  result.Page,
		"size":  result.Size,
	})
}

// GetProject handles GET /admin/projects/:id
func (s *AdminServer) GetProject(ctx echo.Context) error {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid project id")
	}

	project, err := s.svc.Project.GetProject(ctx.Request().Context(), id)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	return response.Success(ctx, adminvo.NewAdminProjectVO(project))
}

// TakedownProject handles PATCH /admin/projects/:id/takedown
func (s *AdminServer) TakedownProject(ctx echo.Context) error {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid project id")
	}

	if err := s.svc.Project.TakedownProject(ctx.Request().Context(), id); err != nil {
		return mapServiceError(ctx, err)
	}

	return response.SuccessMessage(ctx, "操作成功")
}

type reviewProjectRequest struct {
	Status int `json:"status"`
}

// ReviewProject handles PATCH /admin/projects/:id
func (s *AdminServer) ReviewProject(ctx echo.Context) error {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid project id")
	}

	var req reviewProjectRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}

	if req.Status != models.ProjectStatusApproved && req.Status != models.ProjectStatusRejected {
		return response.BadRequest(ctx, fmt.Sprintf("invalid status %d, must be %d (approve) or %d (reject)", req.Status, models.ProjectStatusApproved, models.ProjectStatusRejected))
	}

	if err := s.svc.Project.ReviewProject(ctx.Request().Context(), id, req.Status); err != nil {
		return mapServiceError(ctx, err)
	}

	return response.SuccessMessage(ctx, "操作成功")
}
