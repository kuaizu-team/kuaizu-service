package middleware

import (
	"strings"

	"github.com/labstack/echo/v4"
	adminauth "github.com/kuaizu-team/kuaizu-service/internal/admin/auth"
)

// AdminJWTConfig holds admin JWT middleware configuration
type AdminJWTConfig struct {
	AuthConfig *adminauth.AdminConfig
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

			c.Set("adminID", claims.AdminID)
			c.Set("adminUsername", claims.Username)

			return next(c)
		}
	}
}
