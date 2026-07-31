package handler

import (
	"testing"
	"time"

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

func TestParseInfoCenterEventView(t *testing.T) {
	tests := []struct {
		raw  string
		want infoCenterEventView
		ok   bool
	}{
		{"recommend", infoCenterEventViewRecommend, true},
		{" support ", infoCenterEventViewSupport, true},
		{"", "", false},
		{"other", "", false},
	}

	for _, tt := range tests {
		got, ok := parseInfoCenterEventView(tt.raw)
		if ok != tt.ok || got != tt.want {
			t.Fatalf("parseInfoCenterEventView(%q) = (%q, %v), want (%q, %v)", tt.raw, got, ok, tt.want, tt.ok)
		}
	}
}

func TestInfoCenterSchoolEventParams(t *testing.T) {
	now := time.Date(2026, time.July, 31, 23, 30, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60))

	recommend := infoCenterSchoolEventParams(infoCenterEventViewRecommend, 42, now)
	if recommend.Size != 1 {
		t.Fatalf("recommend size = %d, want 1", recommend.Size)
	}
	if !recommend.SchoolOnly || len(recommend.SchoolIDs) != 1 || recommend.SchoolIDs[0] != 42 {
		t.Fatalf("recommend school filter = %#v", recommend)
	}
	if recommend.RegistrationDeadlineFrom == nil ||
		recommend.RegistrationDeadlineFrom.Format("2006-01-02") != "2026-07-31" {
		t.Fatalf("recommend deadline filter = %v", recommend.RegistrationDeadlineFrom)
	}

	support := infoCenterSchoolEventParams(infoCenterEventViewSupport, 42, now)
	if support.Size != 100 {
		t.Fatalf("support size = %d, want 100", support.Size)
	}
	if !support.SchoolOnly || len(support.SchoolIDs) != 1 || support.SchoolIDs[0] != 42 {
		t.Fatalf("support school filter = %#v", support)
	}
}
