package service

import (
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"strings"
	"testing"
)

func TestEventWebsiteAndNoteValidation(t *testing.T) {
	for _, value := range []string{"https://example.com/event?a=1", "http://example.com", "  "} {
		note := "  个人或团队，分赛道报名  "
		event := &models.Event{Name: "event", OfficialWebsite: &value, ParticipationNote: &note}
		if err := validateEvent(event); err != nil {
			t.Fatal(err)
		}
		if *event.ParticipationNote != "个人或团队，分赛道报名" {
			t.Fatal("note not normalized")
		}
		if strings.TrimSpace(value) == "" && event.OfficialWebsite != nil {
			t.Fatal("empty website must be NULL")
		}
	}
	for _, value := range []string{"javascript:alert(1)", "example.com", "https://user:pass@example.com", "https://example.com/" + strings.Repeat("a", 2048)} {
		if err := validateEvent(&models.Event{Name: "event", OfficialWebsite: &value}); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
	long := strings.Repeat("字", 10001)
	if err := validateEvent(&models.Event{Name: "event", ParticipationNote: &long}); err == nil {
		t.Fatal("long note accepted")
	}
}
