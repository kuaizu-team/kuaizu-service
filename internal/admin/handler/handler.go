package handler

import (
	"errors"
	"strconv"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/kuaizu-team/kuaizu-service/internal/service"
	"github.com/labstack/echo/v4"
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

// --- Permission helpers ---

// currentAdminID returns the logged-in admin's ID from context.
func currentAdminID(ctx echo.Context) int {
	id, _ := ctx.Get("adminID").(int)
	return id
}

// adminRole returns the logged-in admin's role from context.
// Falls back to SuperAdmin (1) for legacy tokens that pre-date the role field.
func adminRole(ctx echo.Context) int {
	if role, ok := ctx.Get("adminRole").(int); ok && role > 0 {
		return role
	}
	return models.AdminRoleSuperAdmin
}

// adminSchoolID returns the logged-in admin's school ID.
// Returns nil when the admin is a super admin (no school binding).
func adminSchoolID(ctx echo.Context) *int {
	schoolID, ok := ctx.Get("adminSchoolID").(int)
	if !ok || schoolID == 0 {
		return nil
	}
	return &schoolID
}

// isSuperAdmin returns true when the current admin has full global access.
func isSuperAdmin(ctx echo.Context) bool {
	return adminRole(ctx) == models.AdminRoleSuperAdmin
}

// requireSchoolAccess returns 403 if role is SchoolAdmin (role=3),
// used to block endpoints that school admins are not allowed to see.
func requireSchoolAccess(ctx echo.Context) error {
	if adminRole(ctx) == models.AdminRoleSchoolAdmin {
		return response.Forbidden(ctx, "权限不足")
	}
	return nil
}
