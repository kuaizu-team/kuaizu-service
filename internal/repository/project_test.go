package repository

import (
	"context"
	"database/sql/driver"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
)

func TestProjectKeywordSearchUsesDegradedMatchingAcrossRequestedFields(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	repo := NewProjectRepository(sqlx.NewDb(db, "capture_user_repo"))
	setCapturedQueryQueue(
		captureQueryResult{columns: []string{"count"}, rows: [][]driver.Value{{int64(0)}}},
		captureQueryResult{columns: []string{"id"}},
	)
	keyword := "比赛"

	projects, total, err := repo.List(context.Background(), ListParams{Page: 1, Size: 10, Keyword: &keyword})
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || len(projects) != 0 {
		t.Fatalf("unexpected result: total=%d projects=%#v", total, projects)
	}

	capturedQuery.Lock()
	queries := append([]string(nil), capturedQuery.queries...)
	args := append([]driver.NamedValue(nil), capturedQuery.args...)
	capturedQuery.Unlock()
	if len(queries) != 2 {
		t.Fatalf("query count = %d, want 2", len(queries))
	}
	for _, query := range queries {
		normalized := normalizeSQL(query)
		for _, want := range []string{
			"p.name LIKE ? ESCAPE '!'",
			"p.description LIKE ? ESCAPE '!'",
			"EXISTS (SELECT 1 FROM school search_school",
			"EXISTS (SELECT 1 FROM project_tag_relation search_ptr",
			"search_pt.status = 1",
			"EXISTS (SELECT 1 FROM project_milestones search_milestone",
			"search_milestone.title LIKE ? ESCAPE '!'",
			"search_milestone.detail_description LIKE ? ESCAPE '!'",
			"EXISTS (SELECT 1 FROM project_members search_member",
			"search_member_school.school_name LIKE ? ESCAPE '!'",
			"search_member_major.major_name LIKE ? ESCAPE '!'",
			"CONCAT(CAST(search_user.grade AS CHAR), '级') LIKE ? ESCAPE '!'",
			"search_member_profile.self_evaluation LIKE ? ESCAPE '!'",
			"search_member_profile.project_experience LIKE ? ESCAPE '!'",
			"CAST(search_member_profile.skill_summary AS CHAR) LIKE ? ESCAPE '!'",
		} {
			if !strings.Contains(normalized, want) {
				t.Fatalf("query missing %q: %s", want, normalized)
			}
		}
	}
	listQuery := normalizeSQL(queries[1])
	for _, want := range []string{
		"CASE WHEN",
		"THEN 4",
		"THEN 3",
		"THEN 2",
		"THEN 1",
		"END DESC, p.created_at DESC",
	} {
		if !strings.Contains(listQuery, want) {
			t.Fatalf("ranking query missing %q: %s", want, listQuery)
		}
	}
	patterns := map[interface{}]bool{}
	for _, arg := range args {
		patterns[arg.Value] = true
	}
	for _, want := range []string{"%比赛%", "%比赛%", "%比%", "%赛%"} {
		if !patterns[want] {
			t.Fatalf("missing search pattern %q in args", want)
		}
	}
}

func TestProjectTodayViewsSortUsesCalendarDayViewsWithStableFallback(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	repo := NewProjectRepository(sqlx.NewDb(db, "capture_user_repo"))
	setCapturedQueryQueue(
		captureQueryResult{columns: []string{"count"}, rows: [][]driver.Value{{int64(0)}}},
		captureQueryResult{columns: []string{"id"}},
	)
	sortBy := "today_views"

	projects, total, err := repo.List(context.Background(), ListParams{Page: 1, Size: 10, SortBy: &sortBy})
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || len(projects) != 0 {
		t.Fatalf("unexpected result: total=%d projects=%#v", total, projects)
	}

	queries, _ := capturedQueriesAndArgs()
	if len(queries) != 2 {
		t.Fatalf("query count = %d, want 2", len(queries))
	}
	query := normalizeSQL(queries[1])
	want := "ORDER BY (SELECT COUNT(*) FROM project_view_log pvl WHERE pvl.project_id = p.id AND pvl.viewed_at >= CURDATE() AND pvl.viewed_at < CURDATE() + INTERVAL 1 DAY AND pvl.duration_ms IS NULL ) DESC, p.created_at DESC, p.id DESC"
	if !strings.Contains(query, want) {
		t.Fatalf("today views ordering missing or unstable: %s", query)
	}
}
