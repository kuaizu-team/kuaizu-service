package service

import "testing"

func TestWelcomeEmailTraceIDUsesCommittedDelivery(t *testing.T) {
	if got := welcomeEmailTraceID(42); got != "welcome-email:42" {
		t.Fatalf("welcomeEmailTraceID(42) = %q", got)
	}
}
