package handler

import (
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/labstack/echo/v4"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestScheduleSaveCompletionCompatibility(t *testing.T) {
	for _, tc := range []struct {
		field      string
		completion interface{}
		result     bool
	}{
		{"", nil, true}, {`,"completed":false`, false, false}, {`,"completed":true`, true, true},
	} {
		t.Run(tc.field, func(t *testing.T) {
			db, m, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			srv := NewServer(repository.New(sqlx.NewDb(db, "sqlmock")), nil)
			m.ExpectExec("UPDATE user_schedule SET title=.*WHERE id=.*AND user_id=").WithArgs("测试", "2026-11-14", false, "09:00", "", "", "2026-11-14", "10:00", tc.completion, int64(9), 7).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectQuery("SELECT completed FROM user_schedule WHERE id=.*AND user_id=").WithArgs(int64(9), 7).WillReturnRows(sqlmock.NewRows([]string{"completed"}).AddRow(tc.result))
			body := `{"title":"测试","date":"2026-11-14","allDay":false,"time":"09:00","endDate":"2026-11-14","endTime":"10:00"` + tc.field + "}"
			e := echo.New()
			req := httptest.NewRequest(http.MethodPut, "/users/me/schedules/9", strings.NewReader(body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.Set("userID", 7)
			c.SetParamNames("id")
			c.SetParamValues("9")
			if err := srv.saveSchedule(c); err != nil {
				t.Fatal(err)
			}
			if rec.Code != 200 {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			if err := m.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestCompletionRejectsMissingBoolean(t *testing.T) {
	for _, body := range []string{`{}`, `{"completed":null}`, `{"completed":"yes"}`} {
		e := echo.New()
		req := httptest.NewRequest(http.MethodPut, "/users/me/schedules/9/completion", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("9")
		if err := (&Server{}).completeSchedule(c); err != nil {
			t.Fatal(err)
		}
		if rec.Code != 400 {
			t.Fatalf("status %d", rec.Code)
		}
	}
}
