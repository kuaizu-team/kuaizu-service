package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
<<<<<<< HEAD
=======
	echomiddleware "github.com/labstack/echo/v4/middleware"
>>>>>>> 4962773a9d48e324fbd164cc3eace0ecfd5c0c67
)

func TestMethodOverrideRoutesMiniProgramPatch(t *testing.T) {
	e := echo.New()
<<<<<<< HEAD
	e.Pre(patchMethodOverride())
=======
	e.Pre(echomiddleware.MethodOverride())
>>>>>>> 4962773a9d48e324fbd164cc3eace0ecfd5c0c67
	paths := []string{
		"/api/v2/project-applications/:id",
		"/api/v2/olive-branches/:id",
		"/api/v2/orders/:id/refund/withdraw",
	}
	for _, path := range paths {
		e.PATCH(path, func(c echo.Context) error {
			return c.NoContent(http.StatusNoContent)
		})
	}

	requests := []string{
		"/api/v2/project-applications/741",
		"/api/v2/olive-branches/52",
		"/api/v2/orders/83/refund/withdraw",
	}
	for _, path := range requests {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.Header.Set(echo.HeaderXHTTPMethodOverride, http.MethodPatch)
		res := httptest.NewRecorder()
		e.ServeHTTP(res, req)

		if res.Code != http.StatusNoContent {
			t.Fatalf("%s status = %d, want %d", path, res.Code, http.StatusNoContent)
		}
	}
}
<<<<<<< HEAD

func TestMethodOverrideOnlyAllowsPatch(t *testing.T) {
	e := echo.New()
	e.Pre(patchMethodOverride())
	const path = "/resource/7"
	e.POST(path, func(c echo.Context) error {
		return c.NoContent(http.StatusAccepted)
	})
	e.PATCH(path, func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	})
	e.GET(path, func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})
	e.PUT(path, func(c echo.Context) error {
		return c.NoContent(http.StatusCreated)
	})
	e.DELETE(path, func(c echo.Context) error {
		return c.NoContent(http.StatusGone)
	})

	tests := []struct {
		name           string
		method         string
		overrideMethod string
		wantStatus     int
	}{
		{name: "post overridden to patch", method: http.MethodPost, overrideMethod: http.MethodPatch, wantStatus: http.StatusNoContent},
		{name: "plain post remains post", method: http.MethodPost, wantStatus: http.StatusAccepted},
		{name: "native patch remains patch", method: http.MethodPatch, wantStatus: http.StatusNoContent},
		{name: "get override is ignored", method: http.MethodPost, overrideMethod: http.MethodGet, wantStatus: http.StatusAccepted},
		{name: "put override is ignored", method: http.MethodPost, overrideMethod: http.MethodPut, wantStatus: http.StatusAccepted},
		{name: "delete override is ignored", method: http.MethodPost, overrideMethod: http.MethodDelete, wantStatus: http.StatusAccepted},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, path, nil)
			if tt.overrideMethod != "" {
				req.Header.Set(echo.HeaderXHTTPMethodOverride, tt.overrideMethod)
			}
			res := httptest.NewRecorder()
			e.ServeHTTP(res, req)

			if res.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", res.Code, tt.wantStatus)
			}
		})
	}
}
=======
>>>>>>> 4962773a9d48e324fbd164cc3eace0ecfd5c0c67
