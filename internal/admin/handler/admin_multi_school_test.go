package handler

import "testing"

func TestValidateAdminSchools(t *testing.T) {
	tests := []struct {
		name    string
		input   []adminSchoolRequest
		wantErr bool
	}{
		{name: "multiple schools", input: []adminSchoolRequest{{SchoolID: 1, CommissionRate: 70}, {SchoolID: 2, CommissionRate: 45.5}}},
		{name: "duplicate school", input: []adminSchoolRequest{{SchoolID: 1, CommissionRate: 20}, {SchoolID: 1, CommissionRate: 30}}, wantErr: true},
		{name: "invalid rate", input: []adminSchoolRequest{{SchoolID: 1, CommissionRate: 100.01}}, wantErr: true},
		{name: "invalid school", input: []adminSchoolRequest{{SchoolID: 0, CommissionRate: 10}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateAdminSchools(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateAdminSchools() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && len(got) != len(tt.input) {
				t.Fatalf("got %d schools, want %d", len(got), len(tt.input))
			}
		})
	}
}

func TestSchoolIDInScope(t *testing.T) {
	schoolID := 20
	if !schoolIDInScope(&schoolID, []int{10, 20, 30}) {
		t.Fatal("expected school to be in multi-school scope")
	}
	if schoolIDInScope(&schoolID, []int{10, 30}) {
		t.Fatal("unexpected school access")
	}
	if schoolIDInScope(nil, []int{20}) {
		t.Fatal("nil school must never be accessible")
	}
}
