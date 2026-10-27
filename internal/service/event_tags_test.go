package service

import (
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"testing"
)

func TestEventTagsValidation(t *testing.T) {
	for _, raw := range []string{`["算法设计","团队协作","科技创新","开发"]`, `["算法设计"," 算法设计 "]`, `[""]`, `["一二三四五六七八九十一二三"]`, `{"tag":"开发"}`, `[1]`} {
		event := &models.Event{Name: "赛事", TagsJSON: &raw}
		if err := validateEvent(event); err == nil {
			t.Errorf("invalid tags accepted: %s", raw)
		}
	}
	for _, raw := range []string{`[]`, `null`, `[" 算法设计 ","团队协作","科技创新"]`} {
		event := &models.Event{Name: "赛事", TagsJSON: &raw}
		if err := validateEvent(event); err != nil {
			t.Fatal(err)
		}
		if tags := event.TagNames(); len(tags) > 0 && tags[0] != "算法设计" {
			t.Fatalf("not trimmed: %v", tags)
		}
	}
}
