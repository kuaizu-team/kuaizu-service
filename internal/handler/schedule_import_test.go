package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestScheduleImportRejectsWholeInvalidBatchBeforeDatabase(t *testing.T) {
	for _, body := range []string{
		`{"items":[]}`,
		`{"items":[{"title":"数学","date":"2026-11-16","allDay":true},{"title":"英语","date":"2026-02-30","allDay":true}]}`,
		`{"items":[{"title":"数学","date":"2026-11-16","allDay":false,"time":"24:00"}]}`,
	} {
		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/users/me/schedules/import", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		recorder := httptest.NewRecorder()
		ctx := e.NewContext(req, recorder)
		ctx.Set("userID", 7)
		// No repository: touching the DB before validating every item would panic.
		if err := (&Server{}).importSchedules(ctx); err != nil {
			t.Fatal(err)
		}
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	}
}
