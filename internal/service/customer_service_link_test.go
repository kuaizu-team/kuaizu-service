package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/wechat"
)

type fakeURLLinkGenerator struct {
	calls int
	url   string
	err   error
	input wechat.URLLinkRequest
}

func (f *fakeURLLinkGenerator) GenerateURLLink(_ context.Context, input wechat.URLLinkRequest) (string, error) {
	f.calls++
	f.input = input
	return f.url, f.err
}

func TestCustomerServiceLinkCachesGeneratedURL(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	generator := &fakeURLLinkGenerator{url: "https://wxaurl.cn/ticket-1"}
	service := NewCustomerServiceLinkService(generator)
	service.now = func() time.Time { return now }

	first, err := service.URL(context.Background())
	if err != nil {
		t.Fatalf("first URL: %v", err)
	}
	second, err := service.URL(context.Background())
	if err != nil {
		t.Fatalf("second URL: %v", err)
	}
	if first != second || first != "https://wxaurl.cn/ticket-1" {
		t.Fatalf("URLs = %q, %q", first, second)
	}
	if generator.calls != 1 {
		t.Fatalf("generator calls = %d, want 1", generator.calls)
	}
	if generator.input.Path != customerServicePagePath || generator.input.ExpireInterval != 30 || generator.input.EnvVersion != "release" {
		t.Fatalf("unexpected input: %#v", generator.input)
	}
}

func TestCustomerServiceLinkUsesValidCacheWhenRefreshFails(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	generator := &fakeURLLinkGenerator{url: "https://wxaurl.cn/ticket-1"}
	service := NewCustomerServiceLinkService(generator)
	service.now = func() time.Time { return now }

	if _, err := service.URL(context.Background()); err != nil {
		t.Fatalf("initial URL: %v", err)
	}
	now = now.Add(urlLinkRefreshAfter + time.Hour)
	generator.err = errors.New("wechat unavailable")

	url, err := service.URL(context.Background())
	if err != nil {
		t.Fatalf("cached URL after refresh failure: %v", err)
	}
	if url != "https://wxaurl.cn/ticket-1" {
		t.Fatalf("cached URL = %q", url)
	}
	if generator.calls != 2 {
		t.Fatalf("generator calls = %d, want 2", generator.calls)
	}
}
