package repository

import "testing"

func TestNormalizeOwnedKeys(t *testing.T) {
	tests := []struct {
		name    string
		keys    []string
		want    int
		wantErr bool
		max     int
	}{
		{name: "accepts owned directory and removes duplicates", keys: []string{"project-images/2026/08/24/a.jpg", "project-images/2026/08/24/a.jpg"}, want: 1, max: 2},
		{name: "rejects another business directory", keys: []string{"milestone-evidence/2026/08/24/a.jpg"}, wantErr: true, max: 1},
		{name: "rejects traversal", keys: []string{"project-images/../avatar.jpg"}, wantErr: true, max: 1},
		{name: "rejects count overflow", keys: []string{"project-images/1", "project-images/2"}, wantErr: true, max: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeOwnedKeys(tc.keys, "project-images/", tc.max)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error=%v wantErr=%v", err, tc.wantErr)
			}
			if err == nil && len(got) != tc.want {
				t.Fatalf("len=%d want=%d", len(got), tc.want)
			}
		})
	}
}
