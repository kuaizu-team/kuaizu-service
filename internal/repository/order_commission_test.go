package repository

import "testing"

func TestCalculateCommissionAmount(t *testing.T) {
	tests := []struct {
		name        string
		actualPaid  []float64
		rate        float64
		wantAmounts []int64
		wantTotal   int64
	}{
		{name: "fractional rate", actualPaid: []float64{100}, rate: 0.01, wantAmounts: []int64{1}, wantTotal: 1},
		{name: "twenty percent", actualPaid: []float64{100}, rate: 20, wantAmounts: []int64{2000}, wantTotal: 2000},
		{name: "full rate", actualPaid: []float64{100}, rate: 100, wantAmounts: []int64{10000}, wantTotal: 10000},
		{name: "round total and keep details balanced", actualPaid: []float64{0.01, 0.01, 0.01}, rate: 50, wantAmounts: []int64{1, 1, 0}, wantTotal: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := make([]settlementOrderRow, len(tt.actualPaid))
			for i, paid := range tt.actualPaid {
				rows[i].ActualPaid = paid
			}
			gotAmounts, gotTotal := calculateCommissionAmounts(rows, tt.rate)
			if gotTotal != tt.wantTotal {
				t.Fatalf("total = %d, want %d", gotTotal, tt.wantTotal)
			}
			for i, got := range gotAmounts {
				if got != tt.wantAmounts[i] {
					t.Fatalf("amount[%d] = %d, want %d", i, got, tt.wantAmounts[i])
				}
			}
		})
	}
}

func TestSettleSchoolPendingOrdersRejectsInvalidCommissionRate(t *testing.T) {
	repo := &OrderRepository{}
	for _, rate := range []float64{0, -1, 100.01} {
		if _, err := repo.SettleSchoolPendingOrders(t.Context(), 1, 1, 2, rate, nil); err == nil {
			t.Fatalf("rate %v: expected validation error", rate)
		}
	}
}
