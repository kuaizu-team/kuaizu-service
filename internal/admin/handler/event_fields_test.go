package handler

import (
	"encoding/json"
	adminvo "github.com/kuaizu-team/kuaizu-service/internal/admin/vo"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
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
