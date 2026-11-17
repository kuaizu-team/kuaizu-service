package models

import "testing"

func TestScheduleOverlapBoundaries(t *testing.T) {
	str := func(v string) *string { return &v }
	item := func(date, start, endDate, end string) Schedule {
		return Schedule{Date: date, Time: str(start), EndDate: str(endDate), EndTime: str(end)}
	}
	a := item("2026-11-17", "09:00", "2026-11-17", "10:00")
	tests := []struct {
		name string
		b    Schedule
		want bool
	}{
		{"adjacent", item("2026-11-17", "10:00", "2026-11-17", "11:00"), false},
		{"contained", item("2026-11-17", "09:30", "2026-11-17", "09:45"), true},
		{"cross midnight", item("2026-11-16", "23:00", "2026-11-17", "09:01"), true},
		{"all day", Schedule{Date: "2026-11-17", AllDay: true}, true},
		{"previous all day", Schedule{Date: "2026-11-16", AllDay: true}, false},
		{"legacy point", Schedule{Date: "2026-11-17", Time: str("09:30")}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if SchedulesOverlap(a, tc.b) != tc.want || SchedulesOverlap(tc.b, a) != tc.want {
				t.Fatal("incorrect overlap")
			}
		})
	}
}
