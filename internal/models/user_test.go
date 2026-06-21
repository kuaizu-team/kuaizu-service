package models

import "testing"

func TestCollaborationLevelThresholds(t *testing.T) {
	cases := []struct {
		score float64
		want  string
	}{
		{0, "较差"},
		{49, "较差"},
		{50, "中等"},
		{84, "中等"},
		{85, "良好"},
		{89, "良好"},
		{90, "优秀"},
		{94, "优秀"},
		{95, "极好"},
		{100, "极好"},
	}

	for _, tc := range cases {
		if got := CollaborationLevel(tc.score); got != tc.want {
			t.Fatalf("CollaborationLevel(%v) = %q, want %q", tc.score, got, tc.want)
		}
	}
}
