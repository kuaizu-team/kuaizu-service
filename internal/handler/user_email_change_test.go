package handler

import "testing"

func TestNormalizedEmail(t *testing.T) {
	email := "  Student@Example.COM "
	if got := normalizedEmail(&email); got != "student@example.com" {
		t.Fatalf("normalizedEmail() = %q, want %q", got, "student@example.com")
	}
	if got := normalizedEmail(nil); got != "" {
		t.Fatalf("normalizedEmail(nil) = %q, want empty", got)
	}
}
