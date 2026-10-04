package handler

import (
	"encoding/csv"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
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
		export bool
		status int
	}{
		{models.AdminRoleSchoolAdmin, `{}`, true, 403},
		{models.AdminRoleEventManager, `{"userIds":[1]}`, false, 403},
		{models.AdminRoleSuperAdmin, `{"userIds":[1,1]}`, false, 400},
		{models.AdminRoleSchoolAdmin, `{"userIds":[]}`, false, 400},
		{models.AdminRoleSuperAdmin, `{"selected":true,"userIds":[]}`, true, 400},
	} {
		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/admin/users/export", strings.NewReader(tc.body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("adminRole", tc.role)
		s := &AdminServer{}
		var err error
		if tc.export {
			err = s.ExportUsers(c)
		} else {
			err = s.BatchApproveUsers(c)
		}
		require.NoError(t, err)
		require.Equal(t, tc.status, rec.Code)
	}
	require.Error(t, validateUserIDs(make([]int, 101), 100))
}

func TestInvalidExportFilterStopsBeforeQuery(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/admin/users/export?talentProfileStatus=99", strings.NewReader(`{}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("adminRole", models.AdminRoleSuperAdmin)
	require.NoError(t, (&AdminServer{}).ExportUsers(c))
	require.Equal(t, 400, rec.Code)
}

func TestExportCSVContainsAllFieldsAndProtectsFormulas(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := &AdminServer{repo: repository.New(sqlx.NewDb(db, "sqlmock"))}
	mock.ExpectQuery("SELECT u.id FROM.*LIMIT 10001").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery("SELECT COALESCE.*ORDER BY u.id").WithArgs(1).WillReturnRows(sqlmock.NewRows([]string{"nickname", "mbti", "school", "major", "grade", "intro", "experience", "score", "auth", "talent", "phone", "wechat", "email", "status"}).AddRow("=HYPERLINK(1)", "INTJ", "学校", "专业", "2023", "第一行\n第二行", "项目,经历", 98.5, 1, 1, "13800000000", "wx", "a@example.test", 2))
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/admin/users/export", strings.NewReader(`{}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("adminRole", models.AdminRoleSuperAdmin)
	require.NoError(t, s.ExportUsers(c))
	require.Equal(t, 200, rec.Code)
	require.Contains(t, rec.Header().Get("Content-Disposition"), "filename*=UTF-8''")
	data := strings.TrimPrefix(rec.Body.String(), "\xef\xbb\xbf")
	records, err := csv.NewReader(strings.NewReader(data)).ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 2)
	require.Len(t, records[0], 15)
	require.Equal(t, "'=HYPERLINK(1)", records[1][0])
	require.Equal(t, "第一行\n第二行", records[1][5])
	require.Equal(t, "项目,经历", records[1][6])
	require.Equal(t, "极好", records[1][7])
	require.Equal(t, "98.50", records[1][8])
	require.Equal(t, "已入驻人才库", records[1][10])
	require.Equal(t, "已毕业", records[1][14])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSelectedExportRejectsForeignSchool(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := &AdminServer{repo: repository.New(sqlx.NewDb(db, "sqlmock"))}
	mock.ExpectQuery("SELECT school_id FROM admin_school_relation").WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"school_id"}).AddRow(10).AddRow(11))
	mock.ExpectQuery("SELECT id,school_id FROM").WithArgs(1, 2).WillReturnRows(sqlmock.NewRows([]string{"id", "school_id"}).AddRow(1, 10).AddRow(2, 99))
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/admin/users/export?schoolId=99", strings.NewReader(`{"selected":true,"userIds":[1,2]}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("adminRole", models.AdminRoleSchoolSuperAdmin)
	c.Set("adminID", 7)
	require.NoError(t, s.ExportUsers(c))
	require.Equal(t, 403, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestExportOverLimitReturnsNoAttachment(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := &AdminServer{repo: repository.New(sqlx.NewDb(db, "sqlmock"))}
	rows := sqlmock.NewRows([]string{"id"})
	for i := 1; i <= 10001; i++ {
		rows.AddRow(i)
	}
	mock.ExpectQuery("SELECT u.id FROM.*LIMIT 10001").WillReturnRows(rows)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/admin/users/export", strings.NewReader(`{}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("adminRole", models.AdminRoleSuperAdmin)
	require.NoError(t, s.ExportUsers(c))
	require.Equal(t, 400, rec.Code)
	require.Empty(t, rec.Header().Get("Content-Disposition"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSafeCSVCellWhitespaceFormula(t *testing.T) {
	for _, value := range []string{"=1", " +1", "\t@SUM(1)", "\n-1"} {
		require.True(t, strings.HasPrefix(safeCSVCell(value), "'"))
	}
	require.Equal(t, "正文\n第二行", safeCSVCell("正文\n第二行"))
}

func TestSchoolSuperExportUsesAllAuthorizedSchoolsAndOverridesQuery(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := &AdminServer{repo: repository.New(sqlx.NewDb(db, "sqlmock"))}
	mock.ExpectQuery("SELECT school_id FROM admin_school_relation").WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"school_id"}).AddRow(10).AddRow(11))
	mock.ExpectQuery("SELECT u.id FROM.*u.school_id IN.*LIMIT 10001").WithArgs(10, 11).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/admin/users/export?schoolId=99", strings.NewReader(`{}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("adminRole", models.AdminRoleSchoolSuperAdmin)
	c.Set("adminID", 7)
	require.NoError(t, s.ExportUsers(c))
	require.Equal(t, 200, rec.Code)
	records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(rec.Body.String(), "\xef\xbb\xbf"))).ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSelectedExportRetainsMissingFieldsAndNonOnlineState(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := &AdminServer{repo: repository.New(sqlx.NewDb(db, "sqlmock"))}
	mock.ExpectQuery("SELECT id,school_id FROM").WithArgs(1).WillReturnRows(sqlmock.NewRows([]string{"id", "school_id"}).AddRow(1, nil))
	mock.ExpectQuery("SELECT u.id FROM.*LIMIT 10001").WithArgs(1).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery("SELECT COALESCE.*ORDER BY u.id").WithArgs(1).WillReturnRows(sqlmock.NewRows([]string{"nickname", "mbti", "school", "major", "grade", "intro", "experience", "score", "auth", "talent", "phone", "wechat", "email", "status"}).AddRow("", "", "", "", "2", "", "", nil, 0, 0, "", "", "", 1))
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/admin/users/export", strings.NewReader(`{"selected":true,"userIds":[1]}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("adminRole", models.AdminRoleSuperAdmin)
	require.NoError(t, s.ExportUsers(c))
	require.Equal(t, 200, rec.Code)
	records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(rec.Body.String(), "\xef\xbb\xbf"))).ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 2)
	require.Equal(t, "2", records[1][4])
	require.Empty(t, records[1][7])
	require.Empty(t, records[1][8])
	require.Equal(t, "否", records[1][9])
	require.Equal(t, "未入驻人才库", records[1][10])
	require.Equal(t, "封禁", records[1][14])
	require.NoError(t, mock.ExpectationsWereMet())
}
