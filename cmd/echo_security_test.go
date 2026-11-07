package cmd

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func TestEchoDefaultRealIPRejectsSpoofedForwardingHeaders(t *testing.T) {
	e := echo.New()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "203.0.113.7:1234"
	r.Header.Set(echo.HeaderXForwardedFor, "198.51.100.8")
	r.Header.Set(echo.HeaderXRealIP, "198.51.100.9")
	if got := e.NewContext(r, httptest.NewRecorder()).RealIP(); got != "203.0.113.7" {
		t.Fatalf("spoofed IP accepted: %s", got)
	}
}

func TestEchoProxyOverwritesSpoofedRealIP(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, r.Header.Get(echo.HeaderXRealIP)) }))
	defer upstream.Close()
	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Use(middleware.Proxy(middleware.NewRoundRobinBalancer([]*middleware.ProxyTarget{{URL: target}})))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "203.0.113.7:1234"
	r.Header.Set(echo.HeaderXRealIP, "198.51.100.9")
	r.Header.Set(echo.HeaderXForwardedFor, "198.51.100.8")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, r)
	if rec.Code != http.StatusOK || rec.Body.String() != "203.0.113.7" {
		t.Fatalf("spoofed upstream IP: status=%d body=%s", rec.Code, rec.Body.String())
	}
}
