package handler

import (
	"encoding/json"
	adminvo "github.com/kuaizu-team/kuaizu-service/internal/admin/vo"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"testing"
)

func TestEventTagsOmittedAndExplicitClear(t *testing.T) {
	stored := `["算法设计","团队协作","科技创新"]`
	existing := &models.Event{TagsJSON: &stored}
	for _, tc := range []struct {
		body  string
		count int
	}{
		{`{"name":"赛事"}`, 3},
		{`{"name":"赛事","tags":null}`, 0},
		{`{"name":"赛事","tags":[]}`, 0},
		{`{"name":"赛事","tags":["开发"]}`, 1},
	} {
		var req adminEventRequest
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(tc.body), &req); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(tc.body), &raw); err != nil {
			t.Fatal(err)
		}
		event, err := buildAdminEventModel(req, models.AdminRoleSuperAdmin, nil)
		if err != nil {
			t.Fatal(err)
		}
		preserveOmittedEventDetails(event, existing, raw)
		admin := adminvo.NewAdminEventVO(event)
		public := event.ToVO()
		if len(admin.Tags) != tc.count || public.Tags == nil || len(*public.Tags) != tc.count {
			t.Fatalf("tag roundtrip: %s", tc.body)
		}
	}
}
