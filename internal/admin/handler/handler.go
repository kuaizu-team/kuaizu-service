package handler

import (
	"errors"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/kuaizu-team/kuaizu-service/internal/service"
)

// AdminServer handles admin API requests
type AdminServer struct {
	repo *repository.Repository
	svc  *service.Services
}

// NewAdminServer creates a new AdminServer instance
func NewAdminServer(repo *repository.Repository, svc *service.Services) *AdminServer {
	return &AdminServer{repo: repo, svc: svc}
}

// mapServiceError maps a service.ServiceError to the appropriate HTTP error response.
func mapServiceError(ctx echo.Context, err error) error {
	var svcErr *service.ServiceError
	if errors.As(err, &svcErr) {
		switch svcErr.Code {
		case service.ErrCodeBadRequest:
			return response.BadRequest(ctx, svcErr.Message)
		case service.ErrCodeNotFound:
			return response.NotFound(ctx, svcErr.Message)
		case service.ErrCodeForbidden:
			return response.Forbidden(ctx, svcErr.Message)
		default:
			return response.Error(ctx, int(svcErr.Code), svcErr.Message)
		}
	}
	return response.InternalError(ctx, err.Error())
}

func parseIDParam(ctx echo.Context, name, label string) (int, error) {
	id, err := strconv.Atoi(ctx.Param(name))
	if err != nil {
		return 0, response.BadRequest(ctx, "invalid "+label+" id")
	}
	return id, nil
}
