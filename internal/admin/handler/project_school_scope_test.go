package handler

import (
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/labstack/echo/v4"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSchoolAdminProjectScope(t *testing.T) {
	for _, tc := range []struct {
		name    string
		school  interface{}
		allowed bool
	}{
		{"own school", 22, true}, {"other school", 23, false}, {"unassigned", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer raw.Close()
			server := &AdminServer{repo: repository.New(sqlx.NewDb(raw, "mysql"))}
			rec := httptest.NewRecorder()
			ctx := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/admin/projects/7", nil), rec)
			ctx.Set("adminRole", models.AdminRoleSchoolAdmin)
			ctx.Set("adminSchoolID", 22)
			mock.ExpectQuery("SELECT school_id FROM project WHERE id=").WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"school_id"}).AddRow(tc.school))
			if err := server.requireProjectAccess(ctx, 7); err != nil {
				t.Fatal(err)
			}
			if (rec.Code == 200) != tc.allowed {
				t.Fatalf("status=%d", rec.Code)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
