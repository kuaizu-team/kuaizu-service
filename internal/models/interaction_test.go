package models

import (
	"encoding/json"
	"testing"
)

func TestFavoriteViewStateIncludesStandardAndLegacyFields(t *testing.T) {
	state := FavoriteViewState{
		ProjectCount: 3, TalentCount: 2, TotalCount: 5,
		Projects: 3, TalentProfiles: 2, Total: 5,
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]int
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]int{
		"projectCount": 3, "talentCount": 2, "totalCount": 5,
		"projects": 3, "talentProfiles": 2, "total": 5,
	} {
		if got[key] != want {
			t.Fatalf("%s = %d, want %d", key, got[key], want)
		}
	}
}
