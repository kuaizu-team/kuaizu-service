package repository

import "testing"

func TestInteractionTables(t *testing.T) {
	tests := []struct {
		target, like, id string
	}{
		{InteractionProject, "project_like", "project_id"},
		{InteractionTalent, "talent_like", "talent_profile_id"},
	}
	for _, tt := range tests {
		like, _, _, id, err := interactionTables(tt.target)
		if err != nil || like != tt.like || id != tt.id {
			t.Fatalf("interactionTables(%q) = %q, %q, %v", tt.target, like, id, err)
		}
	}
	if _, _, _, _, err := interactionTables("bad"); err == nil {
		t.Fatal("expected invalid target error")
	}
}
