package handler

import (
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUserBulkPermissionAndValidation(t *testing.T) {
	for _, tc := range []struct {
		role   int
		body   string
		status int
	}{
		{models.AdminRoleEventManager, `{"userIds":[1]}`, 403},
		{models.AdminRoleSuperAdmin, `{"userIds":[1,1]}`, 400},
		{models.AdminRoleSchoolAdmin, `{"userIds":[]}`, 400},
	} {
		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/admin/users/talent-approve", strings.NewReader(tc.body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("adminRole", tc.role)
		require.NoError(t, (&AdminServer{}).BatchApproveUsers(c))
		require.Equal(t, tc.status, rec.Code)
	}
	require.Error(t, validateUserIDs(make([]int, 101), 100))
}
