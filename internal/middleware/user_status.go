package middleware

import (
	"net/http"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
)

// UserStatusCheck returns a middleware that blocks banned or graduated users from
// accessing authenticated business endpoints.
//
// It runs after JWTAuth (which sets "userID" in the Echo context) and performs a
// lightweight single-row lookup by primary key — acceptable for campus-scale traffic.
//
// Skipped paths:
//   - Unauthenticated requests (no userID in context)
//   - GET /api/v2/users/me — clients need this to learn their own status
//   - GET /api/v2/users/me/certification — informational read-only endpoint
//
// Error codes returned to the client:
//   - 4031: account is banned   (HTTP 403, carries banReason in data)
//   - 4032: account is graduated (HTTP 403)
func UserStatusCheck(repo *repository.Repository) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Skip unauthenticated requests — JWT middleware already handled those.
			userID, ok := c.Get("userID").(int)
			if !ok || userID == 0 {
				return next(c)
			}

			// Allow read-only status-check endpoints so the client can always
			// determine its own account state, even when restricted.
			path := c.Path()
			if c.Request().Method == http.MethodGet &&
				(path == "/api/v2/users/me" || path == "/api/v2/users/me/certification") {
				return next(c)
			}

			user, err := repo.User.GetByID(c.Request().Context(), userID)
			if err != nil || user == nil {
				// Fail open: if we can't determine status, let the request through.
				return next(c)
			}

			switch user.UserStatus {
			case models.UserStatusBanned:
				return c.JSON(http.StatusForbidden, response.Response{
					Code:    4031,
					Message: "您的账号已被封禁",
					Data:    map[string]interface{}{"banReason": user.BanReason},
				})
			case models.UserStatusGraduated:
				return c.JSON(http.StatusForbidden, response.Response{
					Code:    4032,
					Message: "您已经毕业，账号已停用",
				})
			}

			return next(c)
		}
	}
}
