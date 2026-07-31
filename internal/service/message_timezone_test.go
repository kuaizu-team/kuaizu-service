package service

import (
	"testing"
	"time"
)

func TestFormatChinaTime(t *testing.T) {
	tests := []struct {
		name  string
		value time.Time
		want  string
	}{
		{
			name:  "UTC server time is converted to Beijing time",
			value: time.Date(2026, 7, 28, 14, 0, 0, 0, time.UTC),
			want:  "22:00",
		},
		{
			name:  "Beijing time remains unchanged",
			value: time.Date(2026, 7, 28, 22, 0, 0, 0, chinaStandardTime),
			want:  "22:00",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := formatChinaTime(test.value); got != test.want {
				t.Fatalf("formatChinaTime() = %q, want %q", got, test.want)
			}
		})
	}
}
