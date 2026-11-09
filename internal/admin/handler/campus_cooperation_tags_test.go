package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/service"
	"github.com/labstack/echo/v4"
)

func TestCooperationTagSuggestionsAreAuthenticatedAndBounded(t *testing.T) {
	for _, role := range []int{1, 4} {
		raw, m, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		repo := repository.New(sqlx.NewDb(raw, "sqlmock"))
		server := NewAdminServer(repo, &service.Services{CampusCooperation: service.NewCampusCooperationService(repo, nil, nil)})
		m.ExpectQuery("SELECT role,school_id,status FROM admin_user WHERE id=\\?").WithArgs(7).
			WillReturnRows(sqlmock.NewRows([]string{"role", "school_id", "status"}).AddRow(role, nil, 1))
		if role == 1 {
			m.ExpectQuery("(?s)SELECT id,name FROM project_tag").WithArgs("%科研%", "科研%", 12).
				WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(28, "科研"))
		}
		rec := httptest.NewRecorder()
		ctx := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/admin/project-tags?keyword=科研&limit=12", nil), rec)
		ctx.Set("adminID", 7)
		if err := server.SearchCooperationTags(ctx); err != nil {
			t.Fatal(err)
		}
		if role == 1 && (rec.Code != 200 || !strings.Contains(rec.Body.String(), "科研")) {
			t.Fatalf("valid suggestions: %d %s", rec.Code, rec.Body)
		}
		if role == 4 && rec.Code != 403 {
			t.Fatalf("event-only role accessed library: %d", rec.Code)
		}
		if err := m.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		raw.Close()
	}
	for _, limit := range []string{"0", "21", "bad"} {
		rec := httptest.NewRecorder()
		ctx := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/admin/project-tags?limit="+limit, nil), rec)
		if err := (&AdminServer{}).SearchCooperationTags(ctx); err != nil {
			t.Fatal(err)
		}
		if rec.Code != 400 {
			t.Fatalf("unbounded request accepted: %d", rec.Code)
		}
	}
}
