package handler

import (
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFeishuHandlerRejectsEventRoleAndSelectionFilters(t *testing.T) {
	for _, tc := range []struct {
		role   int
		body   string
		status int
	}{
		{4, `{"schoolId":10}`, 403}, {0, `{"schoolId":10}`, 403},
		{1, `{"schoolId":10,"userIds":[1]}`, 400}, {1, `{"schoolId":10,"selected":true}`, 400},
		{1, `{"schoolId":10} {}`, 400}, {1, `{"schoolId":"10"}`, 400},
	} {
		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/admin/users/feishu-sync", strings.NewReader(tc.body))
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("adminRole", tc.role)
		c.Set("adminID", 7)
		require.NoError(t, (&AdminServer{}).StartUserFeishuSync(c))
		require.Equal(t, tc.status, rec.Code)
	}
}
func TestFeishuHandlerUsesAuthoritativeBindingAndRejectsUnbound(t *testing.T) {
	for _, tc := range []struct {
		school    any
		requested int
		status    int
	}{{nil, 0, 403}, {10, 20, 403}, {10, 10, 503}} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		s := &AdminServer{repo: repository.New(sqlx.NewDb(db, "mysql"))}
		mock.ExpectQuery("SELECT id, username, role, school_id, status FROM admin_user").WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "role", "school_id", "status"}).AddRow(7, "admin", 3, tc.school, 1))
		if tc.status == 503 {
			mock.ExpectQuery("SELECT id, school_name.*FROM school").WithArgs(10).WillReturnRows(sqlmock.NewRows([]string{"id", "school_name", "school_code", "province", "city", "district", "created_at", "updated_at"}).AddRow(10, "school", nil, nil, nil, nil, time.Now(), time.Now()))
		}
		e := echo.New()
		rec := httptest.NewRecorder()
		body := `{"schoolId":0}`
		if tc.requested == 20 {
			body = `{"schoolId":20}`
		} else if tc.requested == 10 {
			body = `{"schoolId":10}`
		}
		c := e.NewContext(httptest.NewRequest(http.MethodPost, "/admin/users/feishu-sync", strings.NewReader(body)), rec)
		c.Set("adminID", 7)
		c.Set("adminRole", 3)
		c.Set("adminSchoolID", 99)
		require.NoError(t, s.StartUserFeishuSync(c))
		require.Equal(t, tc.status, rec.Code)
		require.NoError(t, mock.ExpectationsWereMet())
		db.Close()
	}
}
