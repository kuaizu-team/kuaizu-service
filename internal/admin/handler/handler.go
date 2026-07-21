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
		case service.ErrCodeInternal:
			ctx.Logger().Errorf("internal admin service error: %v", err)
			return response.InternalError(ctx, "internal server error")
		default:
			return response.Error(ctx, int(svcErr.Code), svcErr.Message)
		}
	}
	ctx.Logger().Errorf("unhandled admin service error: %v", err)
	return response.InternalError(ctx, "internal server error")
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
// Missing roles stay invalid; AdminJWTAuth rejects legacy tokens before handlers run.
func adminRole(ctx echo.Context) int {
	if role, ok := ctx.Get("adminRole").(int); ok && role > 0 {
		return role
	}
	return 0
}

// adminSchoolID returns the logged-in admin's school ID.
// Returns nil when the admin is a super admin (no school binding).
func adminSchoolID(ctx echo.Context) *int {
	// Role=2 is multi-school; callers must use (*AdminServer).adminSchoolIDs.
	if adminRole(ctx) == models.AdminRoleSchoolSuperAdmin {
		return nil
	}
	schoolID, ok := ctx.Get("adminSchoolID").(int)
	if !ok || schoolID == 0 {
		return nil
	}
	return &schoolID
}

// adminSchoolIDs resolves the authoritative school scope. Role=2 reads all
// positive-rate relations from the database on every request so delegation takes effect
// immediately even when the JWT still contains a legacy schoolId claim.
// A nil slice means unrestricted (platform super admin); an empty slice means
// the school super admin currently has no positive-rate school relation.
func (s *AdminServer) adminSchoolIDs(ctx echo.Context) ([]int, error) {
	switch adminRole(ctx) {
	case models.AdminRoleSuperAdmin:
		return nil, nil
	case models.AdminRoleSchoolSuperAdmin:
		return s.repo.AdminUser.AccessibleSchoolIDs(ctx.Request().Context(), currentAdminID(ctx))
	default:
		if schoolID := adminSchoolID(ctx); schoolID != nil {
			return []int{*schoolID}, nil
		}
		return []int{}, nil
	}
}

func schoolIDInScope(schoolID *int, schoolIDs []int) bool {
	if schoolID == nil {
		return false
	}
	for _, allowed := range schoolIDs {
		if *schoolID == allowed {
			return true
		}
	}
	return false
}

func (s *AdminServer) canAccessSchool(ctx echo.Context, schoolID *int) (bool, error) {
	if adminRole(ctx) == models.AdminRoleSuperAdmin {
		return true, nil
	}
	schoolIDs, err := s.adminSchoolIDs(ctx)
	if err != nil {
		return false, err
	}
	return schoolIDInScope(schoolID, schoolIDs), nil
}

func (s *AdminServer) canEditAdminInScope(ctx echo.Context, target *models.AdminUser) bool {
	if target == nil {
		return false
	}
	role := adminRole(ctx)
	callerID := currentAdminID(ctx)
	if role == models.AdminRoleSuperAdmin {
		return target.ID == callerID || target.Role > models.AdminRoleSuperAdmin
	}
	if role != models.AdminRoleSchoolSuperAdmin {
		return false
	}
	if target.ID == callerID {
		return true
	}
	if target.Role != models.AdminRoleSchoolAdmin && target.Role != models.AdminRoleEventManager {
		return false
	}
	schoolIDs, err := s.adminSchoolIDs(ctx)
	return err == nil && schoolIDInScope(target.SchoolID, schoolIDs)
}

// canEditAdmin reports whether the caller may edit/delete the target admin.
//
// Permission matrix:
//
//	role=1 (super admin)         → can edit self and any role=2/3; cannot edit other role=1.
//	role=2 (school super admin)  → can edit self and same-school role=3; cannot edit role=1/2 (non-self).
//	role=3 (school admin)        → can only edit self; cannot edit anyone else.
func canEditAdmin(callerRole, callerID, targetRole, targetID int, callerSchoolID, targetSchoolID *int) bool {
	switch callerRole {
	case models.AdminRoleSuperAdmin: // role=1
		if targetID == callerID {
			return true // always allowed to edit self
		}
		// may edit role=2 or role=3, but NOT another role=1
		return targetRole > models.AdminRoleSuperAdmin
	case models.AdminRoleSchoolSuperAdmin: // role=2
		if targetID == callerID {
			return true // always allowed to edit self
		}
		// may edit role=3 in the same school
		return (targetRole == models.AdminRoleSchoolAdmin || targetRole == models.AdminRoleEventManager) && schoolIDsMatch(callerSchoolID, targetSchoolID)
	case models.AdminRoleSchoolAdmin: // role=3
		return targetID == callerID // only self
	}
	return false
}

// schoolIDsMatch returns true when both pointers are non-nil and point to the same value.
func schoolIDsMatch(a, b *int) bool {
	return a != nil && b != nil && *a == *b
}

func (s *AdminServer) eventIDForManager(ctx echo.Context) (int, error) {
	var eventID int
	err := s.repo.DB().QueryRowxContext(ctx.Request().Context(), `SELECT id FROM event WHERE admin_id=?`, currentAdminID(ctx)).Scan(&eventID)
	if err != nil {
		return 0, response.Forbidden(ctx, "当前赛事管理员未关联赛事")
	}
	return eventID, nil
}

func (s *AdminServer) requireProjectAccess(ctx echo.Context, projectID int) error {
	if adminRole(ctx) == models.AdminRoleSchoolSuperAdmin {
		var schoolID *int
		if err := s.repo.DB().QueryRowxContext(ctx.Request().Context(), `SELECT school_id FROM project WHERE id=?`, projectID).Scan(&schoolID); err != nil {
			return response.NotFound(ctx, "项目不存在")
		}
		allowed, err := s.canAccessSchool(ctx, schoolID)
		if err != nil || !allowed {
			return response.Forbidden(ctx, "无权访问非负责学校的项目")
		}
		return nil
	}
	if adminRole(ctx) != models.AdminRoleEventManager {
		return nil
	}
	eventID, err := s.eventIDForManager(ctx)
	if err != nil {
		return err
	}
	var exists bool
	if err := s.repo.DB().QueryRowxContext(ctx.Request().Context(), `SELECT EXISTS(SELECT 1 FROM project_event WHERE project_id=? AND event_id=?)`, projectID, eventID).Scan(&exists); err != nil || !exists {
		return response.Forbidden(ctx, "无权访问非本赛事项目")
	}
	return nil
}

// requireEventManagerUserAccess limits read-only user details to people tied
// to one of the current event manager's projects.
func (s *AdminServer) requireEventManagerUserAccess(ctx echo.Context, userID int) error {
	if adminRole(ctx) != models.AdminRoleEventManager {
		return nil
	}
	eventID, err := s.eventIDForManager(ctx)
	if err != nil {
		return err
	}
	var allowed bool
	err = s.repo.DB().QueryRowxContext(ctx.Request().Context(), `
		SELECT EXISTS(
			SELECT 1
			FROM project p
			JOIN project_event pe ON pe.project_id = p.id
			WHERE pe.event_id = ? AND (
				p.creator_id = ?
				OR EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id = p.id AND pm.user_id = ?)
				OR EXISTS(SELECT 1 FROM project_application pa WHERE pa.project_id = p.id AND pa.user_id = ?)
				OR EXISTS(SELECT 1 FROM olive_branch_record obr WHERE obr.related_project_id = p.id AND obr.receiver_id = ?)
			)
		)`, eventID, userID, userID, userID, userID).Scan(&allowed)
	if err != nil || !allowed {
		return response.Forbidden(ctx, "无权查看非关联赛事项目的用户")
	}
	return nil
}

func (s *AdminServer) requireUserSchoolAccess(ctx echo.Context, userID int) error {
	if adminRole(ctx) == models.AdminRoleSuperAdmin || adminRole(ctx) == models.AdminRoleEventManager {
		return nil
	}
	var schoolID *int
	if err := s.repo.DB().QueryRowxContext(ctx.Request().Context(), `SELECT school_id FROM `+"`user`"+` WHERE id=?`, userID).Scan(&schoolID); err != nil {
		return response.NotFound(ctx, "用户不存在")
	}
	allowed, err := s.canAccessSchool(ctx, schoolID)
	if err != nil || !allowed {
		return response.Forbidden(ctx, "无权访问非负责学校的用户")
	}
	return nil
}

func (s *AdminServer) requireOrderSchoolAccess(ctx echo.Context, orderID int) error {
	if adminRole(ctx) != models.AdminRoleSchoolSuperAdmin {
		return nil
	}
	var schoolID *int
	if err := s.repo.DB().QueryRowxContext(ctx.Request().Context(), `
		SELECT u.school_id FROM `+"`order`"+` o JOIN `+"`user`"+` u ON u.id=o.user_id WHERE o.id=?`, orderID).Scan(&schoolID); err != nil {
		return response.NotFound(ctx, "订单不存在")
	}
	allowed, err := s.canAccessSchool(ctx, schoolID)
	if err != nil || !allowed {
		return response.Forbidden(ctx, "无权访问非负责学校的订单")
	}
	return nil
}
