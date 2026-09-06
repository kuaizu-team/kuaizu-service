package models

import (
	"testing"
	"time"
)

func TestPublicEventTimelineOwnsDeadlineAndPreservesStages(t *testing.T) {
	deadline, _ := ParseEventDate("2026-09-04")
	past := deadline.Add(-24 * time.Hour)
	text := "省赛待通知"
	nodes := []EventTimelineNode{
		{ID: 1, Title: "报名截止", NodeTime: &past},
		{ID: 2, Title: "报名截止时间", NodeTime: &past},
		{ID: 3, Title: "决赛报名截止", NodeTime: &past, Description: &text},
		{ID: 4, Title: "省赛", TimeText: &text},
	}
	event := &Event{ID: 42, RegistrationDeadline: &deadline}
	result := PublicEventTimeline(event, nodes)
	if len(result) != 3 || result[0].ID != 3 || result[1].ID != -1 || result[2].ID != 4 {
		t.Fatalf("unexpected nodes: %#v", result)
	}
	if result[1].NodeTime.Format(time.RFC3339) != "2026-09-04T23:59:59+08:00" {
		t.Fatal(result[1].NodeTime)
	}
	if result[0].Description != &text || result[2].TimeText != &text || nodes[0].NodeTime != &past {
		t.Fatal("source information changed")
	}
	if len(PublicEventTimeline(event, result)) != 3 {
		t.Fatal("duplicate synthesized deadline")
	}
	event.RegistrationDeadline = nil
	if got := PublicEventTimeline(event, nodes); len(got) != 2 || got[0].ID != 3 {
		t.Fatalf("cleared deadline/stages: %#v", got)
	}
	if len(CustomEventTimeline(nodes)) != 2 {
		t.Fatal("admin custom timeline includes overall deadline")
	}
}

func TestStageDeadlineTitlesAreNotReserved(t *testing.T) {
	for _, title := range []string{"团队报名截止", "意向报名截止", "决赛报名截止", "校赛报名截止", "网络报名截止"} {
		if IsEventRegistrationDeadlineTitle(title) {
			t.Fatal(title)
		}
	}
	if !IsEventRegistrationDeadlineTitle(" 报名截止时间 ") {
		t.Fatal("trimmed overall title")
	}
}

func TestEventWebsiteVO(t *testing.T) {
	website := "https://example.com/event"
	vo := (&Event{OfficialWebsite: &website}).ToVO()
	if vo.OfficialWebsite == nil || *vo.OfficialWebsite != website {
		t.Fatal("website lost")
	}
}
