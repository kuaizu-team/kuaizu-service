package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	echomiddleware "github.com/labstack/echo/v4/middleware"
)

func TestMethodOverrideRoutesMiniProgramPatch(t *testing.T) {
	e := echo.New()
	e.Pre(echomiddleware.MethodOverride())
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
