package models

import (
	"sort"
	"strings"
	"time"
)

// Only the overall deadline is reserved; stage deadlines must remain intact.
func IsEventRegistrationDeadlineTitle(title string) bool {
	title = strings.TrimSpace(title)
	return title == "报名截止" || title == "报名截止时间"
}

func CustomEventTimeline(nodes []EventTimelineNode) []EventTimelineNode {
	result := make([]EventTimelineNode, 0, len(nodes))
	for _, node := range nodes {
		if !IsEventRegistrationDeadlineTitle(node.Title) {
			result = append(result, node)
		}
	}
	return result
}

// Both public endpoints synthesize the deadline, including for older clients.
// Never mutate stored nodes or infer the overall date from stage deadlines.
func PublicEventTimeline(event *Event, nodes []EventTimelineNode) []EventTimelineNode {
	result := CustomEventTimeline(nodes)
	if event.RegistrationDeadline != nil {
		y, m, d := event.RegistrationDeadline.Date()
		deadline := time.Date(y, m, d, 23, 59, 59, 0, eventRegistrationLocation)
		result = append(result, EventTimelineNode{ID: -1, EventID: event.ID, Title: "报名截止", NodeTime: &deadline})
	}
	sort.SliceStable(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.NodeTime == nil && b.NodeTime != nil {
			return false
		}
		if a.NodeTime != nil && b.NodeTime == nil {
			return true
		}
		if a.NodeTime != nil && !a.NodeTime.Equal(*b.NodeTime) {
			return a.NodeTime.Before(*b.NodeTime)
		}
		if a.SortOrder != b.SortOrder {
			return a.SortOrder < b.SortOrder
		}
		return a.ID < b.ID
	})
	return result
}
