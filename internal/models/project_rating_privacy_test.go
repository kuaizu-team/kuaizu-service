package models

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestProjectMemberRatingStatusJSONOmitsRatingDetails(t *testing.T) {
	now := time.Now()
	payload, err := json.Marshal(ProjectMemberRatingStatus{
		MemberID: 2, Score: float64Ptr(88.5), CanRate: false, RatingFrozen: true, FreezeDays: 5, CooldownDays: 0,
		LastRatedAt: &now, NextRateAt: &now, RatingCount: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	body := string(payload)
	for _, forbidden := range []string{"lastRatedAt", "nextRateAt", "ratingCount"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("client rating status leaked %s: %s", forbidden, body)
		}
	}
	for _, required := range []string{"memberId", "score", "ratingFrozen", "freezeDays", "cooldownDays"} {
		if !strings.Contains(body, required) {
			t.Fatalf("client rating status omitted %s: %s", required, body)
		}
	}
}

func TestProjectMemberRatingResultJSONOmitsRatingDetails(t *testing.T) {
	payload, err := json.Marshal(ProjectMemberRatingResult{
		MemberID: 2, Score: 91, CanRate: false, CooldownDays: 30,
		NextRateAt: time.Now(), RatingCount: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	body := string(payload)
	if strings.Contains(body, "nextRateAt") || strings.Contains(body, "ratingCount") {
		t.Fatalf("client rating result leaked details: %s", body)
	}
}

func float64Ptr(value float64) *float64 { return &value }
