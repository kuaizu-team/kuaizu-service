package handler

import (
	"github.com/kuaizu-team/kuaizu-service/internal/service"
	"github.com/labstack/echo/v4"
	"strconv"
)

func cooperationPageParams(ctx echo.Context) (int, int, error) {
	page, size := 1, 20
	for _, field := range []struct {
		name   string
		target *int
	}{{"page", &page}, {"size", &size}} {
		if value := ctx.QueryParam(field.name); value != "" {
			number, err := strconv.Atoi(value)
			if err != nil || number < 1 || (field.name == "page" && number > 1000000) {
				return 0, 0, service.ErrBadRequest("分页参数无效")
			}
			*field.target = number
		}
	}
	if size > 50 {
		size = 50
	}
	return page, size, nil
}
func cooperationResourceID(ctx echo.Context) (int64, error) {
	id, err := strconv.ParseInt(ctx.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, service.ErrBadRequest("合作ID无效")
	}
	return id, nil
}
func (s *Server) ListCampusCooperations(ctx echo.Context) error {
	page, size, err := cooperationPageParams(ctx)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	data, err := s.svc.CampusCooperation.ClientList(ctx.Request().Context(), GetUserID(ctx), page, size)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return Success(ctx, data)
}
func (s *Server) GetCampusCooperation(ctx echo.Context) error {
	id, err := cooperationResourceID(ctx)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	data, err := s.svc.CampusCooperation.ClientDetail(ctx.Request().Context(), GetUserID(ctx), id)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return Success(ctx, data)
}
func (s *Server) ApplyCampusCooperation(ctx echo.Context) error {
	id, err := cooperationResourceID(ctx)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	data, err := s.svc.CampusCooperation.Apply(ctx.Request().Context(), GetUserID(ctx), id)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return Success(ctx, data)
}
func (s *Server) ListMyCampusCooperationApplications(ctx echo.Context) error {
	page, size, err := cooperationPageParams(ctx)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	list, total, err := s.svc.CampusCooperation.MyApplications(ctx.Request().Context(), GetUserID(ctx), page, size)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return Success(ctx, map[string]interface{}{"list": list, "total": total, "page": page, "size": size})
}
