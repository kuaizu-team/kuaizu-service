package models

import "testing"

func TestIsValidInformationCategoryIncludesTalent(t *testing.T) {
	if !IsValidInformationCategory(InformationCategoryTalent) {
		t.Fatal("talent must be accepted as an information category")
	}
}
