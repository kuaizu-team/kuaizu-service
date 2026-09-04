package models

import (
	"encoding/json"
	"testing"
)

func TestEventTimelineReferenceJSON(t *testing.T) {
	text := "参考：2025年9—11月"
	node := EventTimelineNode{Title: "省赛", TimeText: &text}
	data, err := json.Marshal(node)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]interface{}
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	if value["nodeTime"] != nil || value["timeText"] != text {
		t.Fatalf("unexpected JSON: %s", data)
	}
}
