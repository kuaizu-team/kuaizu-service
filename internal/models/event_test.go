package models

import (
	"testing"
	"time"
)

func TestIsEventRegistrationOpen(t *testing.T) {
	location := time.FixedZone("CST", 8*60*60)
	deadline := time.Date(2026, time.July, 15, 0, 0, 0, 0, location)

	tests := []struct {
		name string
		now  time.Time
		want bool
	}{
		{"start of deadline date", time.Date(2026, time.July, 15, 0, 0, 0, 0, location), true},
		{"last second of deadline date", time.Date(2026, time.July, 15, 23, 59, 59, 0, location), true},
		{"last nanosecond of deadline date", time.Date(2026, time.July, 15, 23, 59, 59, int(time.Second-time.Nanosecond), location), true},
		{"next midnight", time.Date(2026, time.July, 16, 0, 0, 0, 0, location), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsEventRegistrationOpen(&deadline, tt.now); got != tt.want {
				t.Fatalf("IsEventRegistrationOpen() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsEventRegistrationOpenWithoutDeadline(t *testing.T) {
	if !IsEventRegistrationOpen(nil, time.Now()) {
		t.Fatal("an event without a deadline should remain open")
	}
}

func TestEventToVOIncludesRegistrationStatus(t *testing.T) {
	deadline := time.Now().AddDate(0, 0, -1)
	vo := (&Event{RegistrationDeadline: &deadline}).ToVO()

	if vo.IsOpen == nil || *vo.IsOpen {
		t.Fatal("an event whose deadline date has passed should not be open")
	}
	if vo.IsExpired == nil || !*vo.IsExpired {
		t.Fatal("an event whose deadline date has passed should be expired")
	}
}

func TestIsEventRegistrationOpenUsesBusinessTimezone(t *testing.T) {
	location := time.FixedZone("CST", 8*60*60)
	deadlineFromDatabase := time.Date(2026, time.July, 15, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, time.July, 15, 23, 0, 0, 0, location)

	if !IsEventRegistrationOpen(&deadlineFromDatabase, now) {
		t.Fatal("the database calendar date should remain open through the local deadline day")
	}
}
