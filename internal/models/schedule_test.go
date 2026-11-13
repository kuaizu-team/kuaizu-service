package models

import "testing"

func TestScheduleValidation(t *testing.T) {
	for _, tc := range []struct {
		date, clock   string
		allDay, valid bool
	}{
		{"2026-11-13", "09:05", false, true},
		{"2026-02-29", "09:05", false, false},
		{"2026-11-13", "24:00", false, false},
		{"2026-11-13", "9:05", false, false},
		{"2026-11-13", "", false, false},
		{"2026-11-13", "invalid", true, true},
	} {
		t.Run(tc.date+tc.clock, func(t *testing.T) {
			item := Schedule{Title: " test ", Date: tc.date, Time: &tc.clock, AllDay: tc.allDay}
			if err := item.Validate(); (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
			if tc.valid && tc.allDay && item.Time != nil {
				t.Fatal("all-day time must be null")
			}
		})
	}
}
