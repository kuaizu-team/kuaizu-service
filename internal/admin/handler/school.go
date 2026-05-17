package handler

import (
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
)

type schoolDropdownVO struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// ListSchools handles GET /admin/schools — returns all schools for dropdown
func (s *AdminServer) ListSchools(ctx echo.Context) error {
	schools, err := s.repo.School.List(ctx.Request().Context(), repository.SchoolListParams{})
	if err != nil {
		return response.InternalError(ctx, "获取学校列表失败")
	}

	list := make([]schoolDropdownVO, len(schools))
	for i, sc := range schools {
		list[i] = schoolDropdownVO{ID: sc.ID, Name: sc.SchoolName}
	}

	return response.Success(ctx, list)
}
