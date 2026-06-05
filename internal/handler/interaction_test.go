package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestInteractionParamsDefaultsAndValues(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest("GET", "/?page=2&size=25&days=7", nil)
	ctx := e.NewContext(req, httptest.NewRecorder())
	page, size, days := interactionParams(ctx)
	if page != 2 || size != 25 || days != 7 {
		t.Fatalf("unexpected params: %d %d %d", page, size, days)
	}
}
