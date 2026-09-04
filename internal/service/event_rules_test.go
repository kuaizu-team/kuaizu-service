package service

import (
	"testing"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

func TestValidateEventDerivesLegacyCrossRule(t *testing.T) {
	event := &models.Event{Name: "legacy", AllowCrossSchool: 0, AllowCrossMajor: 1}
	if err := validateEvent(event); err != nil {
		t.Fatalf("validateEvent() error = %v", err)
	}
	if event.CrossSchoolMajorRule == nil || *event.CrossSchoolMajorRule != EventCrossRuleAllowMajor {
		t.Fatalf("cross rule = %v, want %q", event.CrossSchoolMajorRule, EventCrossRuleAllowMajor)
	}
}

func TestValidateEventTeamRange(t *testing.T) {
	mode := EventParticipationTeam
	minMembers, maxMembers := 2, 3
	event := &models.Event{
		Name:              "team event",
		ParticipationMode: &mode,
		TeamMinMembers:    &minMembers,
		TeamMaxMembers:    &maxMembers,
	}
	if err := validateEvent(event); err != nil {
		t.Fatalf("validateEvent() error = %v", err)
	}
	if event.TeamMinMembers == nil || event.TeamMaxMembers == nil {
		t.Fatal("team member range was cleared")
	}
}

func TestValidateEventRejectsInvalidTeamRange(t *testing.T) {
	mode := EventParticipationTeam
	minMembers, maxMembers := 3, 2
	event := &models.Event{
		Name:              "invalid team event",
		ParticipationMode: &mode,
		TeamMinMembers:    &minMembers,
		TeamMaxMembers:    &maxMembers,
	}
	if err := validateEvent(event); err == nil {
		t.Fatal("validateEvent() error = nil, want invalid range error")
	}
}

func TestValidateEventBothAndPartialTeamRange(t *testing.T) {
	for _, mode := range []string{EventParticipationTeam, EventParticipationBoth} {
		for _, bounds := range [][2]*int{{nil, nil}, {nil, intPointer(5)}, {intPointer(1), intPointer(3)}} {
			event := &models.Event{Name: "rules", ParticipationMode: &mode, TeamMinMembers: bounds[0], TeamMaxMembers: bounds[1]}
			if err := validateEvent(event); err != nil {
				t.Fatalf("%s: %v", mode, err)
			}
		}
		event := &models.Event{Name: "invalid", ParticipationMode: &mode, TeamMinMembers: intPointer(0)}
		if err := validateEvent(event); err == nil {
			t.Fatal("zero members must be rejected")
		}
	}
}

func intPointer(value int) *int { return &value }
