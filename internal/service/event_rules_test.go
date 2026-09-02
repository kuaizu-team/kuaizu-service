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
