package handler

import (
	"github.com/labstack/echo/v4"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestScheduleSocialRejectsInvalidInputBeforeDatabase(t *testing.T) {
	token := strings.Repeat("a", 64)
	for _, tc := range []struct {
		name, method, path, body string
		action                   func(*Server, echo.Context) error
		param, value             string
	}{
		{"invalid share kind", http.MethodPost, "/shares", `{"kind":"everyone"}`, (*Server).scheduleShares, "", ""},
		{"invalid day", http.MethodPost, "/shares", `{"kind":"day","date":"2026-02-30"}`, (*Server).scheduleShares, "", ""},
		{"invalid token", http.MethodGet, "/view?date=2026-11-17", "", (*Server).scheduleViewShare, "token", "short"},
		{"invalid view date", http.MethodGet, "/view?date=2026-02-30", "", (*Server).scheduleViewShare, "token", token},
		{"missing source", http.MethodPost, "/joint", `{"requestKey":"stable-request-key","item":{"title":"test","date":"2026-11-17","allDay":true}}`, (*Server).scheduleJointCreate, "", ""},
		{"missing end", http.MethodPost, "/joint", `{"projectId":3,"requestKey":"stable-request-key","item":{"title":"test","date":"2026-11-17","allDay":false,"time":"09:00"}}`, (*Server).scheduleJointCreate, "", ""},
		{"two sources", http.MethodPost, "/joint", `{"projectId":3,"token":"` + token + `","requestKey":"stable-request-key"}`, (*Server).scheduleJointCreate, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.Set("userID", 7)
			if tc.param != "" {
				c.SetParamNames(tc.param)
				c.SetParamValues(tc.value)
			}
			if err := tc.action(&Server{}, c); err != nil {
				t.Fatal(err)
			}
			if rec.Code != 400 {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}
