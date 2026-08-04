package cmd

import (
	"reflect"
	"slices"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestCORSConfigUsesScopedDefaults(t *testing.T) {
	t.Setenv("TEST_CORS_ORIGINS", "")
	want := []string{"https://kuaizu.xyz"}
	got := CORSConfig("TEST_CORS_ORIGINS", want)
	if !reflect.DeepEqual(got.AllowOrigins, want) {
		t.Fatalf("origins = %v, want %v", got.AllowOrigins, want)
	}
	if !slices.Contains(got.AllowHeaders, echo.HeaderXHTTPMethodOverride) {
		t.Fatalf("allow headers = %v, missing method override header", got.AllowHeaders)
	}
}

func TestCORSConfigParsesEnvironmentOverride(t *testing.T) {
	t.Setenv("TEST_CORS_ORIGINS", " https://one.example,https://two.example ")
	want := []string{"https://one.example", "https://two.example"}
	defaults := []string{"https://default.example"}
	got := CORSConfig("TEST_CORS_ORIGINS", defaults)
	if !reflect.DeepEqual(got.AllowOrigins, want) {
		t.Fatalf("origins = %v, want %v", got.AllowOrigins, want)
	}
	if !reflect.DeepEqual(defaults, []string{"https://default.example"}) {
		t.Fatalf("defaults mutated: %v", defaults)
	}
}
