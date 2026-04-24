package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kuaizu-team/kuaizu-service/internal/auth"
	"github.com/kuaizu-team/kuaizu-service/internal/requestctx"
)

func TestJWTAuthInjectsOpenIDIntoRequestContext(t *testing.T) {
	jwtCfg := &auth.Config{
		Secret:     "test-secret",
		Issuer:     "test-kuaizu",
		ExpireHour: 1,
	}
	token, _, err := auth.GenerateToken(jwtCfg, 42, "openid-123")
	require.NoError(t, err)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v2/users/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := JWTAuth(&JWTConfig{JWTConfig: jwtCfg})(func(c echo.Context) error {
		assert.Equal(t, 42, c.Get("userID"))
		assert.Equal(t, "openid-123", c.Get("openID"))
		assert.Equal(t, "openid-123", requestctx.OpenIDFromContext(c.Request().Context()))
		return c.NoContent(http.StatusNoContent)
	})

	err = handler(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}
