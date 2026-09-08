package handler

import (
	"context"
	"encoding/json"
	adminvo "github.com/kuaizu-team/kuaizu-service/internal/admin/vo"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/service"
	"testing"
)

func TestAdminEventWebsiteNoteRoundTrip(t *testing.T) {
	var req adminEventRequest
	if err := json.Unmarshal([]byte(`{"name":"event","officialWebsite":" https://example.com ","participationNote":"  赛道说明  ","participationMode":"both","teamMinMembers":1,"teamMaxMembers":null}`), &req); err != nil {
		t.Fatal(err)
	}
	event, err := buildAdminEventModel(req, models.AdminRoleSuperAdmin, nil)
	if err != nil {
		t.Fatal(err)
	}
	vo := adminvo.NewAdminEventVO(event)
	if *vo.OfficialWebsite != "https://example.com" || *vo.ParticipationNote != "赛道说明" || *vo.ParticipationMode != "both" || *vo.TeamMinMembers != 1 || vo.TeamMaxMembers != nil {
		t.Fatalf("fields lost: %#v", vo)
	}
	if err := json.Unmarshal([]byte(`{"name":"event","officialWebsite":null,"participationNote":null,"participationMode":null}`), &req); err != nil {
		t.Fatal(err)
	}
	event, err = buildAdminEventModel(req, models.AdminRoleSuperAdmin, nil)
	if err != nil || event.OfficialWebsite != nil || event.ParticipationNote != nil || event.ParticipationMode != nil {
		t.Fatal("explicit null not accepted")
	}
}

type preservedEventRepo struct {
	repository.EventRepo
	event *models.Event
}

func (r *preservedEventRepo) Update(_ context.Context, event *models.Event) error {
	r.event = event
	return nil
}
func (r *preservedEventRepo) GetByID(context.Context, int) (*models.Event, error) {
	return r.event, nil
}

func TestLegacyEventUpdatePreservesDetailsThroughValidation(t *testing.T) {
	value, rule, mode := "stored detail", "reject_cross_major", "team"
	min, max := 2, 5
	existing := &models.Event{OrganizerName: &value, Description: &value, ResourceURL: &value, QQGroup: &value, CrossSchoolMajorRule: &rule, ParticipationMode: &mode, TeamMinMembers: &min, TeamMaxMembers: &max}
	for _, tt := range []struct {
		body    string
		cleared bool
		min     int
		rule    string
	}{
		{`{"name":"renamed"}`, false, 2, "reject_cross_major"},
		{`{"name":"renamed","description":null,"resourceUrl":"","qqGroup":null}`, true, 2, "reject_cross_major"},
		{`{"name":"renamed","teamMinMembers":3}`, false, 3, "reject_cross_major"},
		{`{"name":"renamed","allowCrossMajor":true}`, false, 2, "allow_cross_major"},
		{`{"name":"renamed","crossSchoolMajorRule":"allow_cross_school_and_major"}`, false, 2, "allow_cross_school_and_major"},
	} {
		var req adminEventRequest
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(tt.body), &req); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(tt.body), &raw); err != nil {
			t.Fatal(err)
		}
		event, err := buildAdminEventModel(req, models.AdminRoleSuperAdmin, nil)
		if err != nil {
			t.Fatal(err)
		}
		preserveOmittedEventDetails(event, existing, raw)
		repo := &preservedEventRepo{}
		updated, err := service.NewEventService(&repository.Repository{Event: repo}).UpdateEvent(context.Background(), event)
		if err != nil {
			t.Fatal(err)
		}
		if updated.OrganizerName == nil || *updated.OrganizerName != value || *updated.TeamMinMembers != tt.min || *updated.TeamMaxMembers != max || *updated.CrossSchoolMajorRule != tt.rule {
			t.Fatalf("lost omitted fields: %+v", updated)
		}
		if tt.cleared {
			if updated.Description != nil || updated.ResourceURL != nil || updated.QQGroup != nil {
				t.Fatal("explicit clearing ignored")
			}
		} else {
			if updated.Description == nil || updated.ResourceURL == nil || updated.QQGroup == nil {
				t.Fatal("omitted details lost")
			}
		}
	}
}

func TestAdminEventOmittedFieldsAndExplicitNull(t *testing.T) {
	website, note, mode := "https://example.com", "保留说明", "both"
	min := 1
	existing := &models.Event{OfficialWebsite: &website, ParticipationNote: &note, ParticipationMode: &mode, TeamMinMembers: &min}
	for _, explicitNull := range []bool{false, true} {
		event := &models.Event{}
		raw := map[string]json.RawMessage{}
		if explicitNull {
			for _, key := range []string{"officialWebsite", "participationNote", "participationMode"} {
				raw[key] = json.RawMessage("null")
			}
		}
		preserveOmittedEventDetails(event, existing, raw)
		if explicitNull {
			if event.OfficialWebsite != nil || event.ParticipationNote != nil || event.ParticipationMode != nil {
				t.Fatal("null must clear fields")
			}
		} else if event.OfficialWebsite != &website || event.ParticipationNote != &note || event.ParticipationMode != &mode || event.TeamMinMembers != &min {
			t.Fatal("omitted fields must preserve stored values")
		}
	}
}
