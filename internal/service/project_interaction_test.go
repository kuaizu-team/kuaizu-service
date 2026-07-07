package service

import "testing"

func TestValidateProjectTags(t *testing.T) {
	valid := []string{"挑战杯", "AI"}
	if err := validateProjectTags(&valid); err != nil {
		t.Fatalf("valid tags rejected: %v", err)
	}
	empty := []string{}
	if err := validateProjectTags(&empty); err != nil {
		t.Fatalf("empty tags rejected: %v", err)
	}
	duplicate := []string{"AI", "AI"}
	if err := validateProjectTags(&duplicate); err == nil {
		t.Fatal("expected duplicate tags error")
	}
	tooMany := []string{"1", "2", "3", "4", "5", "6"}
	if err := validateProjectTags(&tooMany); err == nil {
		t.Fatal("expected tag count error")
	}
	tooLong := []string{"1234567890123"}
	if err := validateProjectTags(&tooLong); err == nil {
		t.Fatal("expected tag length error")
	}
}

func TestValidInteractionType(t *testing.T) {
	for _, kind := range []string{"like", "favorite", "share"} {
		if !validInteractionType(kind) {
			t.Fatalf("%q should be valid", kind)
		}
	}
	for _, kind := range []string{"", "view", "LIKE"} {
		if validInteractionType(kind) {
			t.Fatalf("%q should be invalid", kind)
		}
	}
}
