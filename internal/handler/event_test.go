package handler

import (
	"testing"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

func TestParseEventDateUsesBusinessTimezone(t *testing.T) {
	got, err := models.ParseEventDate("2026-07-15")
	if err != nil {
		t.Fatalf("parseEventDate() error = %v", err)
	}

	if got.Format("2006-01-02 15:04:05") != "2026-07-15 00:00:00" {
		t.Fatalf("ParseEventDate() = %v, want local midnight", got)
	}
	if _, offset := got.Zone(); offset != 8*60*60 {
		t.Fatalf("parseEventDate() UTC offset = %d, want %d", offset, 8*60*60)
	}
}

func TestParseEventDateRejectsDateTime(t *testing.T) {
	if _, err := models.ParseEventDate("2026-07-15T00:00:00"); err == nil {
		t.Fatal("ParseEventDate() should reject a datetime value")
	}
}

func TestBuildEventModelUsesBusinessTimezoneForDeadline(t *testing.T) {
	deadline := "2026-07-15"
	event, err := buildEventModel(eventRequest{Name: "test", RegistrationDeadline: &deadline})
	if err != nil {
		t.Fatalf("buildEventModel() error = %v", err)
	}
	if event.RegistrationDeadline == nil {
		t.Fatal("buildEventModel() deadline is nil")
	}
	if _, offset := event.RegistrationDeadline.Zone(); offset != 8*60*60 {
		t.Fatalf("deadline UTC offset = %d, want %d", offset, 8*60*60)
	}
}
