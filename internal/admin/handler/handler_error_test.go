package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/kuaizu-team/kuaizu-service/internal/service"
	"github.com/labstack/echo/v4"
)

func TestAdminMapServiceErrorRedactsInternalDetails(t *testing.T) {
	e := echo.New()
	rec := httptest.NewRecorder()
	ctx := e.NewContext(httptest.NewRequest(http.MethodGet, "/admin/sms/send", nil), rec)

	if err := mapServiceError(ctx, service.ErrInternal("message center returned sensitive details")); err != nil {
		t.Fatalf("mapServiceError returned error: %v", err)
	}

	var body response.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if rec.Code != http.StatusInternalServerError || body.Code != http.StatusInternalServerError {
		t.Fatalf("status/code = %d/%d, want %d/%d", rec.Code, body.Code, http.StatusInternalServerError, http.StatusInternalServerError)
	}
	if body.Message != "internal server error" {
		t.Fatalf("message = %q, want generic internal error", body.Message)
	}
}

func TestAdminMapServiceErrorPreservesBusinessMessage(t *testing.T) {
	e := echo.New()
	rec := httptest.NewRecorder()
	ctx := e.NewContext(httptest.NewRequest(http.MethodGet, "/admin/projects", nil), rec)

	if err := mapServiceError(ctx, service.ErrForbidden("permission denied")); err != nil {
		t.Fatalf("mapServiceError returned error: %v", err)
	}

	var body response.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if rec.Code != http.StatusForbidden || body.Message != "permission denied" {
		t.Fatalf("status/message = %d/%q, want %d/%q", rec.Code, body.Message, http.StatusForbidden, "permission denied")
	}
}
