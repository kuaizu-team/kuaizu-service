package middleware

import (
	"strings"

	"github.com/kuaizu-team/kuaizu-service/internal/auth"
	"github.com/kuaizu-team/kuaizu-service/internal/requestctx"
	"github.com/labstack/echo/v4"
)

// JWTConfig holds JWT middleware configuration
type JWTConfig struct {
	JWTConfig *auth.Config
	Skipper   func(c echo.Context) bool
}

// DefaultJWTConfig returns default configuration
func DefaultJWTConfig() *JWTConfig {
	return &JWTConfig{
		JWTConfig: auth.DefaultConfig(),
		Skipper:   nil,
	}
}

// JWTAuth returns a JWT authentication middleware
func JWTAuth(config *JWTConfig) echo.MiddlewareFunc {
	if config == nil {
		config = DefaultJWTConfig()
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Skip authentication if skipper returns true
			if config.Skipper != nil && config.Skipper(c) {
				return next(c)
			}

			// Extract token from Authorization header
			authHeader := c.Request().Header.Get("Authorization")
			if authHeader == "" {
				return echo.NewHTTPError(401, "missing authorization header")
			}

			// Check Bearer scheme
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				return echo.NewHTTPError(401, "invalid authorization header format")
			}

			tokenString := parts[1]

			// Parse and validate token
			claims, err := auth.ParseToken(config.JWTConfig, tokenString)
			if err != nil {
				return echo.NewHTTPError(401, "invalid or expired token")
			}

			// Set user info in context BEFORE calling next
			// This ensures the logger middleware can access these values
			c.Set("userID", claims.UserID)
			c.Set("openID", claims.OpenID)
			reqCtx := requestctx.WithOpenID(c.Request().Context(), claims.OpenID)
			c.SetRequest(c.Request().WithContext(reqCtx))

			// Call next handler and preserve any error
			if err := next(c); err != nil {
				// Store error in context for logger to access
				c.Set("handlerError", err)
				return err
			}

			return nil
		}
	}
}
