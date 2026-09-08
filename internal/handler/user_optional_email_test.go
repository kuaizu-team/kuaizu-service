package handler

import (
	"github.com/labstack/echo/v4"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOptionalEmailUserUpdate(t *testing.T) {
	for _, tc := range []struct {
		body             string
		want             string
		present, invalid bool
	}{
		{`{"schoolId":1,"majorId":2,"grade":2026}`, "", false, false},
		{`{"email":""}`, "", true, false},
		{`{"email":"  "}`, "", true, false},
		{`{"email":" student@example.com "}`, "student@example.com", true, false},
		{`{"email":"invalid"}`, "", false, true},
	} {
		t.Run(tc.body, func(t *testing.T) {
			request := httptest.NewRequest("PUT", "/users/me", strings.NewReader(tc.body))
			request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			dto, err := bindOptionalEmailUserUpdate(echo.New().NewContext(request, httptest.NewRecorder()))
			if (err != nil) != tc.invalid {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.invalid {
				return
			}
			if (dto.Email != nil) != tc.present {
				t.Fatal("email presence changed")
			}
			if dto.Email != nil && string(*dto.Email) != tc.want {
				t.Fatalf("email = %q", *dto.Email)
			}
			if !tc.present && (dto.SchoolId == nil || *dto.SchoolId != 1 || dto.Grade == nil || *dto.Grade != 2026) {
				t.Fatal("academic fields lost")
			}
		})
	}
}
