package handler

import (
	adminauth "github.com/kuaizu-team/kuaizu-service/internal/admin/auth"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Login handles POST /admin/auth/login
func (s *AdminServer) Login(ctx echo.Context) error {
	var req loginRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}

	if req.Username == "" || req.Password == "" {
		return response.BadRequest(ctx, "username and password are required")
	}

	admin, err := s.repo.AdminUser.GetByUsername(ctx.Request().Context(), req.Username)
	if err != nil {
		return response.InternalError(ctx, "failed to query admin user")
	}
	if admin == nil {
		return response.Unauthorized(ctx, "invalid username or password")
	}

	if admin.Status == models.AdminUserStatusDisabled {
		return response.Forbidden(ctx, "account is disabled")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(req.Password)); err != nil {
		return response.Unauthorized(ctx, "invalid username or password")
	}

	// Resolve school info for campus admins
	var schoolName *string
	schoolIDInt := 0
	if admin.SchoolID != nil {
		schoolIDInt = *admin.SchoolID
		// SchoolName is already joined in GetByUsername
		schoolName = admin.SchoolName
	}

	config := adminauth.DefaultAdminConfig()
	token, expiresIn, err := adminauth.GenerateAdminToken(config, admin.ID, admin.Username, admin.Role, schoolIDInt)
	if err != nil {
		return response.InternalError(ctx, "failed to generate token")
	}

	return response.Success(ctx, map[string]interface{}{
		"token":      token,
		"expiresIn":  expiresIn,
		"id":         admin.ID,
		"username":   admin.Username,
		"nickname":   admin.Nickname,
		"role":       admin.Role,
		"schoolId":   admin.SchoolID,
		"schoolName": schoolName,
	})
}
