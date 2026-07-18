package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kuaizu-team/kuaizu-service/internal/service"
	"github.com/kuaizu-team/kuaizu-service/internal/wechat"
	"github.com/labstack/echo/v4"
)

type fixedURLLinkGenerator struct {
	url string
}

func (f fixedURLLinkGenerator) GenerateURLLink(_ context.Context, _ wechat.URLLinkRequest) (string, error) {
	return f.url, nil
}

func TestOpenCustomerServiceRedirectsToWeChatURLLink(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/open/customer-service", nil)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)

	server := NewServer(nil, &service.Services{
		CustomerServiceLink: service.NewCustomerServiceLinkService(
			fixedURLLinkGenerator{url: "https://wxaurl.cn/test-ticket"},
		),
	})

	if err := server.OpenCustomerService(ctx); err != nil {
		t.Fatalf("OpenCustomerService: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	if location := rec.Header().Get("Location"); location != "https://wxaurl.cn/test-ticket" {
		t.Fatalf("Location = %q", location)
	}
	if cacheControl := rec.Header().Get("Cache-Control"); cacheControl != "no-store" {
		t.Fatalf("Cache-Control = %q", cacheControl)
	}
}
