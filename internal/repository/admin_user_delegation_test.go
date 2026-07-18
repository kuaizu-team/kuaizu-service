package repository

import "testing"

func TestAddCommissionRatesPreservesExistingShare(t *testing.T) {
	if got := addCommissionRates(70, 30); got != 100 {
		t.Fatalf("addCommissionRates(70, 30) = %v, want 100", got)
	}
	if got := addCommissionRates(33.33, 16.67); got != 50 {
		t.Fatalf("addCommissionRates(33.33, 16.67) = %v, want 50", got)
	}
}
