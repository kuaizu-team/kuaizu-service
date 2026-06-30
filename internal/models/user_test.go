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

func TestDisplayNickname(t *testing.T) {
	tests := []struct {
		name string
		in   *string
		want string
	}{
		{name: "nil", want: DefaultUserNickname},
		{name: "blank", in: stringPtr("  "), want: DefaultUserNickname},
		{name: "legacy", in: stringPtr("匿名用户"), want: DefaultUserNickname},
		{name: "custom", in: stringPtr("  小明  "), want: "小明"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := *DisplayNickname(tt.in); got != tt.want {
				t.Fatalf("DisplayNickname() = %q, want %q", got, tt.want)
			}
		})
	}
}

func stringPtr(value string) *string { return &value }
