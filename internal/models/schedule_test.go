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

func TestScheduleEndValidation(t *testing.T) {
	text := func(s string) *string { return &s }
	for _, tc := range []struct {
		name, date, clock string
		wantError         bool
	}{
		{"same minute", "2026-11-14", "09:00", true},
		{"earlier", "2026-11-14", "08:59", true},
		{"later", "2026-11-14", "10:00", false},
		{"next day", "2026-11-15", "00:00", false},
		{"invalid date", "2026-02-30", "10:00", true},
		{"invalid clock", "2026-11-14", "24:00", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := Schedule{Title: "测试", Date: "2026-11-14", Time: text("09:00"), EndDate: text(tc.date), EndTime: text(tc.clock)}
			if err := item.Validate(); (err != nil) != tc.wantError {
				t.Fatalf("validation=%v", err)
			}
		})
	}
	item := Schedule{Title: "旧日程", Date: "2026-11-14", Time: text("09:00")}
	if err := item.Validate(); err != nil {
		t.Fatal(err)
	}
	item.EndDate = text(item.Date)
	if err := item.Validate(); err == nil {
		t.Fatal("partial end accepted")
	}
	item.AllDay = true
	item.Completed = true
	if err := item.Validate(); err != nil || item.Time != nil || item.EndDate != nil || item.EndTime != nil || !item.Completed {
		t.Fatalf("all-day normalization: %+v %v", item, err)
	}
}
