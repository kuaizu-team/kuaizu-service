package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPublicAndNestedEventsNeverExposeProtectedResources(t *testing.T) {
	secret, website := "protected-fixture", "https://example.com"
	event := Event{Name: "event", ResourceURL: &secret, QQGroup: &secret, OfficialWebsite: &website}
	project := Project{Events: []Event{event}}
	for _, value := range []interface{}{event.ToVO(), project.ToVO(), project.ToDetailVO()} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), secret) {
			t.Fatal("protected resource escaped through public VO")
		}
		if !strings.Contains(string(encoded), website) {
			t.Fatal("public website lost")
		}
	}
	if event.ResourceURL == nil || event.QQGroup == nil {
		t.Fatal("redaction mutated stored model")
	}
}
