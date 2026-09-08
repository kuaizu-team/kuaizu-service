package repository

import (
	"context"
	"database/sql/driver"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
)

func TestProjectListUsesEventFallbackBeforePagination(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	repo := NewProjectRepository(sqlx.NewDb(db, "capture_user_repo"))
	setCapturedQueryQueue(
		captureQueryResult{
			columns: []string{"id", "name"},
			rows: [][]driver.Value{
				{int64(5), "中国国际大学生创新大赛"},
				{int64(6), "挑战杯"},
			},
		},
		captureQueryResult{columns: []string{"count"}, rows: [][]driver.Value{{int64(0)}}},
		captureQueryResult{columns: []string{"id"}},
	)

	projects, total, err := repo.List(context.Background(), ListParams{
		Page: 1, Size: 10, Direction: intPointer(2), EventIDs: []int{5, 6},
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || len(projects) != 0 {
		t.Fatalf("unexpected result: total=%d projects=%#v", total, projects)
	}

	queries, _ := capturedQueriesAndArgs()
	if len(queries) != 3 {
		t.Fatalf("query count = %d, want 3", len(queries))
	}
	countQuery := normalizeSQL(queries[1])
	listQuery := normalizeSQL(queries[2])
	for _, query := range []string{countQuery, listQuery} {
		if !strings.Contains(query, "selected_pe.event_id IN (?,?)") || !strings.Contains(query, "p.name REGEXP ?") {
			t.Fatalf("query does not combine explicit and degraded event matching: %s", query)
		}
	}
	if !strings.Contains(listQuery, "ORDER BY CASE WHEN EXISTS") || !strings.Contains(listQuery, "SELECT COUNT(DISTINCT selected_pe.event_id)") {
		t.Fatalf("event relevance does not precede pagination order: %s", listQuery)
	}
}

func intPointer(value int) *int {
	return &value
}

func TestBuildProjectEventFilterSQLCoversRequestedFieldsAndRanking(t *testing.T) {
	filter := buildProjectEventFilterSQL([]selectedProjectEvent{
		{ID: 5, Name: "中国国际大学生创新大赛"},
		{ID: 9, Name: "挑战杯"},
	})
	combined := normalizeSQL(filter.Predicate + " " + filter.SelectScores())
	for _, want := range []string{
		"selected_pe.event_id IN (?,?)",
		"p.name REGEXP ?",
		"p.description REGEXP ?",
		"project_tag_relation event_search_ptr",
		"project_milestones event_search_milestone",
		"event_search_milestone.detail_description REGEXP ?",
		"JOIN event event_search_event",
		"JOIN event_timeline_node event_search_node",
		"THEN 4",
		"THEN 3",
		"THEN 2",
		"THEN 1",
		"AS event_relations",
		"AS event_matches",
		"AS event_characters",
	} {
		if !strings.Contains(combined, want) {
			t.Fatalf("event filter missing %q: %s", want, combined)
		}
	}
	args := filter.OrderArgs()
	if len(args) == 0 {
		t.Fatal("event filter score args are empty")
	}
	foundFullName, foundPair, foundCharacter := false, false, false
	for _, arg := range args {
		pattern, ok := arg.(string)
		if !ok {
			continue
		}
		if strings.Contains(pattern, "挑战杯") {
			foundFullName = true
		}
		if strings.Contains(pattern, "挑战") {
			foundPair = true
		}
		if pattern == "挑" {
			foundCharacter = true
		}
	}
	if !foundFullName || !foundPair || !foundCharacter {
		t.Fatalf("missing degraded match patterns: full=%v pair=%v character=%v", foundFullName, foundPair, foundCharacter)
	}
}

func TestUniquePositiveIntsKeepsInputOrder(t *testing.T) {
	got := uniquePositiveInts([]int{9, 0, 5, 9, -1, 7})
	want := []int{9, 5, 7}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unique IDs = %#v, want %#v", got, want)
		}
	}
}
