package middleware

import (
	"context"
	"net/http"
	"regexp"
	"strings"

	adminauth "github.com/kuaizu-team/kuaizu-service/internal/admin/auth"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/labstack/echo/v4"
)

type AdminAuthStateStore interface {
	GetAuthStateByID(ctx context.Context, id int) (*models.AdminUser, error)
}

// AdminJWTConfig holds admin JWT middleware configuration
type AdminJWTConfig struct {
	AuthConfig *adminauth.AdminConfig
	AdminUsers AdminAuthStateStore
	Skipper    func(c echo.Context) bool
}

// DefaultAdminJWTConfig returns default admin JWT middleware configuration
func DefaultAdminJWTConfig() *AdminJWTConfig {
	return &AdminJWTConfig{
		AuthConfig: adminauth.DefaultAdminConfig(),
		Skipper:    nil,
	}
}

// AdminJWTAuth returns an admin JWT authentication middleware
func AdminJWTAuth(config *AdminJWTConfig) echo.MiddlewareFunc {
	if config == nil {
		config = DefaultAdminJWTConfig()
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if config.Skipper != nil && config.Skipper(c) {
				return next(c)
			}

			authHeader := c.Request().Header.Get("Authorization")
			if authHeader == "" {
				return echo.NewHTTPError(401, "missing authorization header")
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				return echo.NewHTTPError(401, "invalid authorization header format")
			}

			claims, err := adminauth.ParseAdminToken(config.AuthConfig, parts[1])
			if err != nil {
				return echo.NewHTTPError(401, "invalid or expired token")
			}
			if !validAdminRole(claims.Role) {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid admin role; please sign in again")
			}
			if config.AdminUsers == nil {
				c.Logger().Error("admin authorization state store is not configured")
				return echo.NewHTTPError(http.StatusServiceUnavailable, "administrator authentication is temporarily unavailable")
			}

			admin, err := config.AdminUsers.GetAuthStateByID(c.Request().Context(), claims.AdminID)
			if err != nil {
				c.Logger().Errorf("load current administrator authorization state: %v", err)
				return echo.NewHTTPError(http.StatusServiceUnavailable, "administrator authentication is temporarily unavailable")
			}
			if admin == nil || admin.Status != models.AdminUserStatusEnabled || !validAdminRole(admin.Role) {
				return echo.NewHTTPError(http.StatusUnauthorized, "administrator account is unavailable; please sign in again")
			}

			schoolID := 0
			if (admin.Role == models.AdminRoleSchoolAdmin ||
				admin.Role == models.AdminRoleEventManager) && admin.SchoolID != nil {
				schoolID = *admin.SchoolID
			}
			c.Set("adminID", admin.ID)
			c.Set("adminUsername", admin.Username)
			c.Set("adminRole", admin.Role)
			c.Set("adminSchoolID", schoolID)
			if admin.Role == models.AdminRoleEventManager && !eventManagerRouteAllowed(c.Request().Method, c.Request().URL.Path) {
				return echo.NewHTTPError(http.StatusForbidden, "赛事管理员仅可访问数据看板和项目大厅")
			}

			return next(c)
		}
	}
}

func validAdminRole(role int) bool {
	switch role {
	case models.AdminRoleSuperAdmin,
		models.AdminRoleSchoolSuperAdmin,
		models.AdminRoleSchoolAdmin,
		models.AdminRoleEventManager:
		return true
	default:
		return false
	}
}

var eventManagerProjectPath = regexp.MustCompile(`^/admin/projects/[0-9]+$`)
var eventManagerRestorePath = regexp.MustCompile(`^/admin/projects/[0-9]+/restore$`)
var eventManagerProjectReadPath = regexp.MustCompile(`^/admin/projects/[0-9]+/(applications|olive-branches|activity-summary)$`)
var eventManagerPermanentDeletePath = regexp.MustCompile(`^/admin/projects/[0-9]+/permanent$`)
var eventManagerUserDetailPath = regexp.MustCompile(`^/admin/users/[0-9]+$`)

func eventManagerRouteAllowed(method, path string) bool {
	if method == http.MethodGet && (path == "/admin/auth/me" || path == "/admin/dashboard/stats" || path == "/admin/projects" || eventManagerProjectPath.MatchString(path) || eventManagerProjectReadPath.MatchString(path) || eventManagerUserDetailPath.MatchString(path)) {
		return true
	}
	if method == http.MethodPatch && (eventManagerProjectPath.MatchString(path) || eventManagerRestorePath.MatchString(path)) {
		return true
	}
	if method == http.MethodDelete && eventManagerPermanentDeletePath.MatchString(path) {
		return true
	}
	return false
}
