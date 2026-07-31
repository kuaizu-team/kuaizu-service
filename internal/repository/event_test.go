package repository

import (
	"context"
	"database/sql/driver"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
)

func captureEventListQuery(t *testing.T, params EventListParams) string {
	t.Helper()
	db := openCaptureDB(t)
	defer db.Close()
	repo := NewEventRepository(sqlx.NewDb(db, "capture_user_repo"))
	setCapturedQueryQueue(
		captureQueryResult{columns: []string{"count"}, rows: [][]driver.Value{{int64(0)}}},
		captureQueryResult{columns: []string{"id"}},
	)

	if _, _, err := repo.List(context.Background(), params); err != nil {
		t.Fatal(err)
	}
	queries, _ := capturedQueriesAndArgs()
	if len(queries) != 2 {
		t.Fatalf("query count = %d, want 2", len(queries))
	}
	return normalizeSQL(queries[1])
}

func TestEventListKeepsPublicDefaultOrder(t *testing.T) {
	query := captureEventListQuery(t, EventListParams{Page: 1, Size: 10})
	want := "ORDER BY CASE WHEN e.registration_deadline IS NULL THEN 1 ELSE 0 END ASC, e.registration_deadline ASC, e.display_order DESC, e.created_at DESC, e.id DESC"
	if !strings.Contains(query, want) {
		t.Fatalf("public event order changed: %s", query)
	}
}

func TestEventListUsesExplicitAdminOrder(t *testing.T) {
	query := captureEventListQuery(t, EventListParams{
		Page: 1, Size: 10, SortBy: "displayOrder", Order: "asc",
	})
	want := "ORDER BY CASE WHEN e.display_order IS NULL THEN 1 ELSE 0 END ASC, e.display_order ASC, e.id DESC"
	if !strings.Contains(query, want) {
		t.Fatalf("explicit event order missing: %s", query)
	}
}

func TestEventListCanSortByProjectCount(t *testing.T) {
	query := captureEventListQuery(t, EventListParams{
		Page: 1, Size: 10, SortBy: "projectCount", Order: "asc",
	})
	want := "ORDER BY project_count ASC, e.id DESC"
	if !strings.Contains(query, want) {
		t.Fatalf("project count order missing: %s", query)
	}
}

func TestEventListProjectCountUsesAdminSchoolScope(t *testing.T) {
	query := captureEventListQuery(t, EventListParams{
		Page: 1, Size: 10, SchoolIDs: []int{22, 23}, ProjectSchoolIDs: []int{22, 23},
	})
	for _, want := range []string{
		"LEFT JOIN project p ON p.id = pe.project_id AND p.school_id IN (?, ?)",
		"COALESCE(COUNT(DISTINCT p.id), 0) AS project_count",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("school-scoped project count missing %q: %s", want, query)
		}
	}
	_, args := capturedQueriesAndArgs()
	if len(args) != 6 ||
		args[0].Value != int64(22) || args[1].Value != int64(23) ||
		args[2].Value != int64(22) || args[3].Value != int64(23) {
		t.Fatalf("args = %#v, want project scope followed by event scope", args)
	}
}

func TestEventGetByIDIncludesProjectCount(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	repo := NewEventRepository(sqlx.NewDb(db, "capture_user_repo"))
	setCapturedQuery([]string{"id", "project_count"}, [][]driver.Value{{int64(42), int64(3)}})

	event, err := repo.GetByID(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if event == nil || event.ID != 42 || event.ProjectCount != 3 {
		t.Fatalf("unexpected event: %#v", event)
	}
	queries, args := capturedQueriesAndArgs()
	if len(queries) != 1 {
		t.Fatalf("query count = %d, want 1", len(queries))
	}
	query := normalizeSQL(queries[0])
	want := "(SELECT COUNT(DISTINCT pe.project_id) FROM project_event pe WHERE pe.event_id = event.id) AS project_count"
	if !strings.Contains(query, want) {
		t.Fatalf("project count missing from event detail query: %s", query)
	}
	if len(args) != 1 || args[0].Value != int64(42) {
		t.Fatalf("args = %#v, want event id 42", args)
	}
}

func TestEventGetByIDProjectCountUsesAdminSchoolScope(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	repo := NewEventRepository(sqlx.NewDb(db, "capture_user_repo"))
	setCapturedQuery([]string{"id", "project_count"}, [][]driver.Value{{int64(42), int64(2)}})

	event, err := repo.GetByIDWithProjectSchoolIDs(context.Background(), 42, []int{22, 23})
	if err != nil {
		t.Fatal(err)
	}
	if event == nil || event.ProjectCount != 2 {
		t.Fatalf("unexpected event: %#v", event)
	}
	queries, args := capturedQueriesAndArgs()
	query := normalizeSQL(queries[0])
	want := "JOIN project p ON p.id = pe.project_id AND p.school_id IN (?, ?)"
	if !strings.Contains(query, want) {
		t.Fatalf("school-scoped project count missing: %s", query)
	}
	if len(args) != 3 || args[0].Value != int64(22) || args[1].Value != int64(23) || args[2].Value != int64(42) {
		t.Fatalf("args = %#v, want school ids followed by event id", args)
	}
}

func TestEventListCanRestrictSchoolAdminToSchoolEvents(t *testing.T) {
	query := captureEventListQuery(t, EventListParams{
		Page: 1, Size: 10, SchoolIDs: []int{22}, SchoolOnly: true,
	})
	want := "e.level = 'school' AND e.school_id IN (?)"
	if !strings.Contains(query, want) {
		t.Fatalf("school-only event filter missing: %s", query)
	}
	if strings.Contains(query, "COALESCE(e.level,'') <> 'school'") {
		t.Fatalf("school-only list still includes non-school events: %s", query)
	}
}
